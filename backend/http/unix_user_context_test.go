package http

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/filebrowser/backend/common/settings"
	"github.com/gtsteffaniak/filebrowser/backend/database/users"
)

func TestGetUnixUserIdentityFromMap_DirectMap(t *testing.T) {
	tmpDir := t.TempDir()
	mapPath := filepath.Join(tmpDir, "user-map.json")
	content := `{"alice":{"uid":1001,"gid":1002}}`
	if err := os.WriteFile(mapPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write map file: %v", err)
	}

	identity, found, err := getUnixUserIdentityFromMap(mapPath, "alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected mapping to be found")
	}
	if identity.UID == nil || *identity.UID != 1001 {
		t.Fatalf("unexpected UID: %+v", identity.UID)
	}
	if identity.GID == nil || *identity.GID != 1002 {
		t.Fatalf("unexpected GID: %+v", identity.GID)
	}
	if identity.UID == nil || identity.GID == nil {
		t.Fatalf("unexpected identity: %+v", identity)
	}
}

func TestGetUnixUserIdentityFromMap_EnvelopeMap(t *testing.T) {
	tmpDir := t.TempDir()
	mapPath := filepath.Join(tmpDir, "user-map.json")
	content := `{"users":{"bob":{"uid":2001,"gid":2002}}}`
	if err := os.WriteFile(mapPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write map file: %v", err)
	}

	identity, found, err := getUnixUserIdentityFromMap(mapPath, "bob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected mapping to be found")
	}
	if identity.UID == nil || *identity.UID != 2001 {
		t.Fatalf("unexpected UID: %+v", identity.UID)
	}
	if identity.GID == nil || *identity.GID != 2002 {
		t.Fatalf("unexpected GID: %+v", identity.GID)
	}
	if identity.UID == nil || identity.GID == nil {
		t.Fatalf("unexpected identity: %+v", identity)
	}
}

func TestGetUnixUserIdentityFromMap_UIDOnly(t *testing.T) {
	tmpDir := t.TempDir()
	mapPath := filepath.Join(tmpDir, "user-map.json")
	content := `{"charlie":{"uid":3001}}`
	if err := os.WriteFile(mapPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write map file: %v", err)
	}

	identity, found, err := getUnixUserIdentityFromMap(mapPath, "charlie")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected mapping to be found")
	}
	if identity.UID == nil || *identity.UID != 3001 {
		t.Fatalf("unexpected UID: %+v", identity.UID)
	}
	if identity.GID != nil {
		t.Fatalf("expected GID to be nil, got: %+v", identity.GID)
	}
}

func TestGetUnixUserIdentityFromMap_GIDOnly(t *testing.T) {
	tmpDir := t.TempDir()
	mapPath := filepath.Join(tmpDir, "user-map.json")
	content := `{"dana":{"gid":4002}}`
	if err := os.WriteFile(mapPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write map file: %v", err)
	}

	identity, found, err := getUnixUserIdentityFromMap(mapPath, "dana")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected mapping to be found")
	}
	if identity.GID == nil || *identity.GID != 4002 {
		t.Fatalf("unexpected GID: %+v", identity.GID)
	}
	if identity.UID != nil {
		t.Fatalf("expected UID to be nil, got: %+v", identity.UID)
	}
}

func TestBuildUnixIdentityArgs(t *testing.T) {
	uid := 1001
	gid := 1002

	args, err := buildUnixIdentityArgs(unixUserIdentity{UID: &uid})
	if err != nil {
		t.Fatalf("unexpected error for uid-only: %v", err)
	}
	if len(args) != 2 || args[0] != "--uid" || args[1] != "1001" {
		t.Fatalf("unexpected args for uid-only: %v", args)
	}

	args, err = buildUnixIdentityArgs(unixUserIdentity{GID: &gid})
	if err != nil {
		t.Fatalf("unexpected error for gid-only: %v", err)
	}
	if len(args) != 2 || args[0] != "--gid" || args[1] != "1002" {
		t.Fatalf("unexpected args for gid-only: %v", args)
	}

	args, err = buildUnixIdentityArgs(unixUserIdentity{UID: &uid, GID: &gid})
	if err != nil {
		t.Fatalf("unexpected error for uid+gid: %v", err)
	}
	if len(args) != 4 {
		t.Fatalf("unexpected args length for uid+gid: %v", args)
	}

	_, err = buildUnixIdentityArgs(unixUserIdentity{})
	if err == nil {
		t.Fatal("expected error when both uid and gid are missing")
	}
}

func TestEnforceUnixUserContextPolicy_NoFallbackReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	mapPath := filepath.Join(tmpDir, "user-map.json")
	helperPath := filepath.Join(tmpDir, "helper")

	if err := os.WriteFile(mapPath, []byte(`{"users":{}}`), 0o644); err != nil {
		t.Fatalf("failed to write map file: %v", err)
	}
	if err := os.WriteFile(helperPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to write helper file: %v", err)
	}

	original := settings.Config
	t.Cleanup(func() {
		settings.Config = original
	})

	settings.Config = settings.Settings{}
	settings.Config.Server.Filesystem.UnixUserContext.Enabled = true
	settings.Config.Server.Filesystem.UnixUserContext.FallbackToServiceUser = false
	settings.Config.Server.Filesystem.UnixUserContext.HelperPath = helperPath
	settings.Config.Server.Filesystem.UnixUserContext.UserMapFile = mapPath
	settings.Config.Server.Filesystem.UnixUserContext.HelperTimeoutMs = 1000

	rc := &requestContext{user: &users.User{Username: "missing-user"}}
	err := enforceUnixUserContextPolicy("resource.download", rc)
	if err == nil {
		t.Fatal("expected error when mapping is missing and fallback is disabled")
	}
}

func TestEnforceUnixUserContextPolicy_FallbackAllowsRequest(t *testing.T) {
	tmpDir := t.TempDir()
	mapPath := filepath.Join(tmpDir, "user-map.json")
	helperPath := filepath.Join(tmpDir, "helper")

	if err := os.WriteFile(mapPath, []byte(`{"users":{}}`), 0o644); err != nil {
		t.Fatalf("failed to write map file: %v", err)
	}
	if err := os.WriteFile(helperPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to write helper file: %v", err)
	}

	original := settings.Config
	t.Cleanup(func() {
		settings.Config = original
	})

	settings.Config = settings.Settings{}
	settings.Config.Server.Filesystem.UnixUserContext.Enabled = true
	settings.Config.Server.Filesystem.UnixUserContext.FallbackToServiceUser = true
	settings.Config.Server.Filesystem.UnixUserContext.HelperPath = helperPath
	settings.Config.Server.Filesystem.UnixUserContext.UserMapFile = mapPath
	settings.Config.Server.Filesystem.UnixUserContext.HelperTimeoutMs = 1000

	rc := &requestContext{user: &users.User{Username: "missing-user"}}
	if err := enforceUnixUserContextPolicy("resource.download", rc); err != nil {
		t.Fatalf("expected fallback to allow request, got error: %v", err)
	}
}
