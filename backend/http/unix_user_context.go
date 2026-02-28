package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/gtsteffaniak/filebrowser/backend/common/settings"
	"github.com/gtsteffaniak/go-logger/logger"
)

type unixUserIdentity struct {
	UID *int `json:"uid,omitempty"`
	GID *int `json:"gid,omitempty"`
}

type unixUserMapEnvelope struct {
	Users map[string]unixUserIdentity `json:"users"`
}

type unixContextResolution struct {
	Active    bool
	Operation string
	Username  string
	Identity  unixUserIdentity
	Config    settings.UnixUserContext
}

func enforceUnixUserContextPolicy(operation string, d *requestContext) error {
	_, err := resolveUnixUserContext(operation, d)
	return err
}

func resolveUnixUserContext(operation string, d *requestContext) (unixContextResolution, error) {
	ctxCfg := settings.Config.Server.Filesystem.UnixUserContext
	if !ctxCfg.Enabled {
		return unixContextResolution{Active: false, Operation: operation, Config: ctxCfg}, nil
	}

	if d == nil || d.user == nil || d.user.Username == "" {
		if ctxCfg.FallbackToServiceUser {
			logger.Warningf("unix user context enabled, but request user is unavailable for operation %s; using service account fallback", operation)
			return unixContextResolution{Active: false, Operation: operation, Config: ctxCfg}, nil
		}
		return unixContextResolution{}, fmt.Errorf("unix user context enabled, but request user is unavailable")
	}

	if ctxCfg.RequireRootForHelper && os.Geteuid() != 0 {
		if ctxCfg.FallbackToServiceUser {
			logger.Warningf("unix user context enabled for user %s, but process is not root; using service account fallback for operation %s", d.user.Username, operation)
			return unixContextResolution{Active: false, Operation: operation, Username: d.user.Username, Config: ctxCfg}, nil
		}
		return unixContextResolution{}, fmt.Errorf("unix user context requires root privileges")
	}

	identity, found, err := getUnixUserIdentityFromMap(ctxCfg.UserMapFile, d.user.Username)
	if err != nil {
		if ctxCfg.FallbackToServiceUser {
			logger.Warningf("unix user context map resolution failed for user %s (%v); using service account fallback for operation %s", d.user.Username, err, operation)
			return unixContextResolution{Active: false, Operation: operation, Username: d.user.Username, Config: ctxCfg}, nil
		}
		return unixContextResolution{}, fmt.Errorf("unable to resolve unix user context: %w", err)
	}
	if !found {
		if ctxCfg.FallbackToServiceUser {
			logger.Warningf("unix user context has no mapping for user %s; using service account fallback for operation %s", d.user.Username, operation)
			return unixContextResolution{Active: false, Operation: operation, Username: d.user.Username, Config: ctxCfg}, nil
		}
		return unixContextResolution{}, fmt.Errorf("no unix user mapping found for user %s", d.user.Username)
	}
	if identity.UID == nil && identity.GID == nil {
		if ctxCfg.FallbackToServiceUser {
			logger.Warningf("unix user context mapping for user %s has neither uid nor gid; using service account fallback for operation %s", d.user.Username, operation)
			return unixContextResolution{Active: false, Operation: operation, Username: d.user.Username, Config: ctxCfg}, nil
		}
		return unixContextResolution{}, fmt.Errorf("unix user mapping for user %s must specify at least one of uid or gid", d.user.Username)
	}

	if ctxCfg.HelperPath == "" {
		if ctxCfg.FallbackToServiceUser {
			logger.Warningf("unix user context helper path is empty for user %s; using service account fallback for operation %s", d.user.Username, operation)
			return unixContextResolution{Active: false, Operation: operation, Username: d.user.Username, Identity: identity, Config: ctxCfg}, nil
		}
		return unixContextResolution{}, fmt.Errorf("unix user context helper path is required")
	}

	helperInfo, err := os.Stat(ctxCfg.HelperPath)
	if err != nil {
		if ctxCfg.FallbackToServiceUser {
			logger.Warningf("unix user context helper path is invalid for user %s (%v); using service account fallback for operation %s", d.user.Username, err, operation)
			return unixContextResolution{Active: false, Operation: operation, Username: d.user.Username, Identity: identity, Config: ctxCfg}, nil
		}
		return unixContextResolution{}, fmt.Errorf("unix user context helper path is invalid: %w", err)
	}

	if helperInfo.Mode()&0o111 == 0 {
		if ctxCfg.FallbackToServiceUser {
			logger.Warningf("unix user context helper is not executable for user %s; using service account fallback for operation %s", d.user.Username, operation)
			return unixContextResolution{Active: false, Operation: operation, Username: d.user.Username, Identity: identity, Config: ctxCfg}, nil
		}
		return unixContextResolution{}, fmt.Errorf("unix user context helper is not executable")
	}

	if ctxCfg.AuditEffectiveIdentity {
		uidText := "unset"
		gidText := "unset"
		if identity.UID != nil {
			uidText = strconv.Itoa(*identity.UID)
		}
		if identity.GID != nil {
			gidText = strconv.Itoa(*identity.GID)
		}
		logger.Infof("unix user context resolved operation=%s username=%s uid=%s gid=%s helper=%s", operation, d.user.Username, uidText, gidText, ctxCfg.HelperPath)
	}

	return unixContextResolution{
		Active:    true,
		Operation: operation,
		Username:  d.user.Username,
		Identity:  identity,
		Config:    ctxCfg,
	}, nil
}

func runUnixContextHelper(resolution unixContextResolution, helperOperation string, stdin io.Reader, args ...string) error {
	if !resolution.Active {
		return nil
	}

	timeout := time.Duration(resolution.Config.HelperTimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	baseArgs := []string{
		"--operation", helperOperation,
	}
	identityArgs, err := buildUnixIdentityArgs(resolution.Identity)
	if err != nil {
		return err
	}
	baseArgs = append(identityArgs, baseArgs...)
	cmdArgs := append(baseArgs, args...)

	cmd := exec.CommandContext(ctx, resolution.Config.HelperPath, cmdArgs...)
	cmd.Stdin = stdin
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helper operation %s failed: %w (output=%s)", helperOperation, err, string(output))
	}

	return nil
}

func buildUnixIdentityArgs(identity unixUserIdentity) ([]string, error) {
	args := make([]string, 0, 4)
	if identity.UID != nil {
		args = append(args, "--uid", strconv.Itoa(*identity.UID))
	}
	if identity.GID != nil {
		args = append(args, "--gid", strconv.Itoa(*identity.GID))
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("unix user mapping must specify at least one of uid or gid")
	}
	return args, nil
}

func getUnixUserIdentityFromMap(mapFilePath string, username string) (unixUserIdentity, bool, error) {
	if mapFilePath == "" {
		return unixUserIdentity{}, false, fmt.Errorf("userMapFile is empty")
	}

	raw, err := os.ReadFile(mapFilePath)
	if err != nil {
		return unixUserIdentity{}, false, err
	}

	var envelope unixUserMapEnvelope
	if err := json.Unmarshal(raw, &envelope); err == nil && len(envelope.Users) > 0 {
		identity, ok := envelope.Users[username]
		return identity, ok, nil
	}

	direct := map[string]unixUserIdentity{}
	if err := json.Unmarshal(raw, &direct); err != nil {
		return unixUserIdentity{}, false, fmt.Errorf("unable to parse userMapFile JSON: %w", err)
	}

	identity, ok := direct[username]
	return identity, ok, nil
}
