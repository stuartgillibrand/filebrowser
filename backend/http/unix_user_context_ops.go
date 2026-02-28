package http

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gtsteffaniak/filebrowser/backend/adapters/fs/files"
	"github.com/gtsteffaniak/filebrowser/backend/adapters/fs/fileutils"
	"github.com/gtsteffaniak/filebrowser/backend/common/utils"
	"github.com/gtsteffaniak/filebrowser/backend/database/access"
	"github.com/gtsteffaniak/filebrowser/backend/database/share"
	"github.com/gtsteffaniak/filebrowser/backend/indexing"
	"github.com/gtsteffaniak/go-logger/logger"
)

func deleteFilesWithUnixContext(d *requestContext, source, absPath string, isDir bool) error {
	resolution, err := resolveUnixUserContext("resource.delete.exec", d)
	if err != nil {
		return err
	}
	if !resolution.Active {
		return files.DeleteFiles(source, absPath, isDir)
	}

	err = runUnixContextHelper(resolution, "remove-all", nil, absPath)
	if err != nil {
		if resolution.Config.FallbackToServiceUser {
			logger.Warningf("unix helper delete failed for %s; using service fallback: %v", absPath, err)
			return files.DeleteFiles(source, absPath, isDir)
		}
		return err
	}

	idx := indexing.GetIndex(source)
	if idx == nil || idx.Config.DisableIndexing {
		return nil
	}

	if isDir {
		_ = files.RefreshIndex(source, absPath, true, false)
	}
	return files.RefreshIndex(source, filepath.Dir(absPath), true, false)
}

func writeDirectoryWithUnixContext(d *requestContext, opts utils.FileOptions) error {
	resolution, err := resolveUnixUserContext("resource.mkdir.exec", d)
	if err != nil {
		return err
	}
	if !resolution.Active {
		return files.WriteDirectory(opts)
	}

	idx := indexing.GetIndex(opts.Source)
	if idx == nil {
		return fmt.Errorf("could not get index: %v", opts.Source)
	}
	realPath, _, _ := idx.GetRealPath(opts.Path)
	perm := strconv.FormatUint(uint64(fileutils.PermDir), 8)

	err = runUnixContextHelper(resolution, "mkdir-all", nil, realPath, perm)
	if err != nil {
		if resolution.Config.FallbackToServiceUser {
			logger.Warningf("unix helper mkdir failed for %s; using service fallback: %v", realPath, err)
			return files.WriteDirectory(opts)
		}
		return err
	}

	if err := files.RefreshIndex(idx.Name, opts.Path, true, true); err != nil {
		return err
	}
	parentPath := filepath.Dir(opts.Path)
	if parentPath != "." && parentPath != "/" && parentPath != opts.Path {
		if err := files.RefreshIndex(idx.Name, parentPath, true, false); err != nil {
			logger.Debugf("Could not refresh parent directory %s: %v", parentPath, err)
		}
	}

	return nil
}

func writeFileWithUnixContext(d *requestContext, source, path string, in io.Reader) error {
	resolution, err := resolveUnixUserContext("resource.write.exec", d)
	if err != nil {
		return err
	}
	if !resolution.Active {
		return files.WriteFile(source, path, in)
	}

	idx := indexing.GetIndex(source)
	if idx == nil {
		return fmt.Errorf("could not get index: %v", source)
	}
	realPath, _, _ := idx.GetRealPath(path)
	realPath = strings.TrimRight(realPath, "/")

	if err := os.MkdirAll(filepath.Dir(realPath), fileutils.PermDir); err != nil {
		return err
	}

	tmpDir := filepath.Join(config.Server.CacheDir, "unix-user-context")
	if err := os.MkdirAll(tmpDir, fileutils.PermDir); err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp(tmpDir, "write-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err = io.Copy(tmpFile, in); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err = tmpFile.Close(); err != nil {
		return err
	}

	perm := strconv.FormatUint(uint64(fileutils.PermFile), 8)
	err = runUnixContextHelper(resolution, "write-file-from", nil, tmpPath, realPath, perm)
	if err != nil {
		if resolution.Config.FallbackToServiceUser {
			logger.Warningf("unix helper write failed for %s; using service fallback: %v", realPath, err)
			fallbackReader, openErr := os.Open(tmpPath)
			if openErr != nil {
				return openErr
			}
			defer fallbackReader.Close()
			return files.WriteFile(source, path, fallbackReader)
		}
		return err
	}

	if err := files.RefreshIndex(source, path, false, false); err != nil {
		return err
	}
	parentPath := filepath.Dir(path)
	if parentPath != "." && parentPath != "/" && parentPath != path {
		if err := files.RefreshIndex(source, parentPath, true, false); err != nil {
			logger.Debugf("Could not refresh parent directory %s: %v", parentPath, err)
		}
	}

	return nil
}

func moveResourceWithUnixContext(d *requestContext, isSrcDir bool, sourceIndex, destIndex, realsrc, realdst string, s *share.Storage, a *access.Storage) error {
	resolution, err := resolveUnixUserContext("resource.move.exec", d)
	if err != nil {
		return err
	}
	if !resolution.Active {
		return files.MoveResource(isSrcDir, sourceIndex, destIndex, realsrc, realdst, s, a)
	}

	err = runUnixContextHelper(resolution, "move", nil, realsrc, realdst)
	if err != nil {
		if resolution.Config.FallbackToServiceUser {
			logger.Warningf("unix helper move failed %s -> %s; using service fallback: %v", realsrc, realdst, err)
			return files.MoveResource(isSrcDir, sourceIndex, destIndex, realsrc, realdst, s, a)
		}
		return err
	}

	srcIdx := indexing.GetIndex(sourceIndex)
	dstIdx := indexing.GetIndex(destIndex)
	if srcIdx != nil && !srcIdx.Config.DisableIndexing {
		go files.RefreshIndex(sourceIndex, filepath.Dir(realsrc), true, false) //nolint:errcheck
	}
	if dstIdx != nil && !dstIdx.Config.DisableIndexing {
		if isSrcDir {
			go files.RefreshIndex(destIndex, realdst, true, true) //nolint:errcheck
		}
		go files.RefreshIndex(destIndex, filepath.Dir(realdst), true, false) //nolint:errcheck
	}
	if s != nil && srcIdx != nil && dstIdx != nil {
		go s.UpdateShares(srcIdx.Path, srcIdx.MakeIndexPath(realsrc, isSrcDir), dstIdx.Path, dstIdx.MakeIndexPath(realdst, isSrcDir)) //nolint:errcheck
	}
	if a != nil && srcIdx != nil && dstIdx != nil && srcIdx.Path == dstIdx.Path {
		go a.UpdateRules(srcIdx.Path, srcIdx.MakeIndexPath(realsrc, isSrcDir), dstIdx.MakeIndexPath(realdst, isSrcDir)) //nolint:errcheck
	}

	return nil
}

func copyResourceWithUnixContext(d *requestContext, isSrcDir bool, sourceIndex, destIndex, realsrc, realdst string) error {
	resolution, err := resolveUnixUserContext("resource.copy.exec", d)
	if err != nil {
		return err
	}
	if !resolution.Active {
		return files.CopyResource(isSrcDir, sourceIndex, destIndex, realsrc, realdst)
	}

	err = runUnixContextHelper(resolution, "copy", nil, realsrc, realdst)
	if err != nil {
		if resolution.Config.FallbackToServiceUser {
			logger.Warningf("unix helper copy failed %s -> %s; using service fallback: %v", realsrc, realdst, err)
			return files.CopyResource(isSrcDir, sourceIndex, destIndex, realsrc, realdst)
		}
		return err
	}

	dstIdx := indexing.GetIndex(destIndex)
	if dstIdx != nil && !dstIdx.Config.DisableIndexing {
		if isSrcDir {
			go files.RefreshIndex(destIndex, realdst, true, true) //nolint:errcheck
		}
		go files.RefreshIndex(destIndex, filepath.Dir(realdst), true, false) //nolint:errcheck
	}

	return nil
}
