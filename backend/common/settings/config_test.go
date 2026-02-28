package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestInitialize(t *testing.T) {
	type args struct {
		configFile string
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Initialize(tt.args.configFile)
		})
	}
}

func Test_setDefaults(t *testing.T) {
	tests := []struct {
		name string
		want Settings
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := setDefaults(true); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("setDefaults() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigLoadChanged(t *testing.T) {
	// Create isolated test directory
	testDir := t.TempDir()
	validContent, err := os.ReadFile("./validConfig.yaml")
	if err != nil {
		t.Fatalf("failed to read validConfig.yaml: %v", err)
	}
	configFile := filepath.Join(testDir, "config.yaml")
	if err = os.WriteFile(configFile, validContent, 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	defaultConfig := setDefaults(true)
	err = loadConfigWithDefaults(configFile, true)
	if err != nil {
		t.Fatalf("error loading config file: %v", err)
	}
	// Use go-cmp to compare the two structs
	if diff := cmp.Diff(defaultConfig, Config); diff == "" {
		t.Errorf("No change when there should have been (-want +got):\n%s", diff)
	}
}

func TestConfigLoadEnvVars(t *testing.T) {
	// Create isolated test directory
	testDir := t.TempDir()
	validContent, err := os.ReadFile("./validConfig.yaml")
	if err != nil {
		t.Fatalf("failed to read validConfig.yaml: %v", err)
	}
	configFile := filepath.Join(testDir, "config.yaml")
	if err = os.WriteFile(configFile, validContent, 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	defaultConfig := setDefaults(true)
	expectedKey := "MYKEY"
	// mock environment variables
	os.Setenv("FILEBROWSER_ONLYOFFICE_SECRET", expectedKey)
	err = loadConfigWithDefaults(configFile, true)
	if err != nil {
		t.Fatalf("error loading config file: %v", err)
	}
	if Config.Integrations.OnlyOffice.Secret != expectedKey {
		t.Errorf("Expected OnlyOffice.Secret to be '%v', got '%s'", expectedKey, Config.Integrations.OnlyOffice.Secret)
	}
	// Use go-cmp to compare the two structs
	if diff := cmp.Diff(defaultConfig, Config); diff == "" {
		t.Errorf("No change when there should have been (-want +got):\n%s", diff)
	}
}

func TestConfigLoadSpecificValues(t *testing.T) {
	// Create isolated test directory
	testDir := t.TempDir()
	validContent, err := os.ReadFile("./validConfig.yaml")
	if err != nil {
		t.Fatalf("failed to read validConfig.yaml: %v", err)
	}
	configFile := filepath.Join(testDir, "config.yaml")
	if err = os.WriteFile(configFile, validContent, 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	defaultConfig := setDefaults(true)
	err = loadConfigWithDefaults(configFile, true)
	if err != nil {
		t.Fatalf("error loading config file: %v", err)
	}
	testCases := []struct {
		fieldName string
		globalVal interface{}
		newVal    interface{}
	}{
		{"Server.Database", Config.Server.Database, defaultConfig.Server.Database},
	}

	for _, tc := range testCases {
		if tc.globalVal == tc.newVal {
			t.Errorf("Differences should have been found:\nConfig.%s: %v \nSetConfig: %v \n", tc.fieldName, tc.globalVal, tc.newVal)
		}
	}
}

func TestInvalidConfig(t *testing.T) {
	// Create isolated test directory
	testDir := t.TempDir()
	invalidContent, err := os.ReadFile("./invalidConfig.yaml")
	if err != nil {
		t.Fatalf("failed to read invalidConfig.yaml: %v", err)
	}
	configFile := filepath.Join(testDir, "config.yaml")
	if err = os.WriteFile(configFile, invalidContent, 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	err = loadConfigWithDefaults(configFile, true)
	// Config loads successfully but validation should catch missing sources
	if err == nil {
		err = ValidateConfig(Config)
		if err == nil {
			t.Fatal("expected validation error for config with missing required sources, got nil")
		}
	}
}

func TestValidateConfig_UnixUserContextEnabledRequiresFiles(t *testing.T) {
	tmpDir := t.TempDir()
	helperPath := filepath.Join(tmpDir, "helper")
	userMapPath := filepath.Join(tmpDir, "user-map.json")

	if err := os.WriteFile(helperPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to create helper file: %v", err)
	}
	if err := os.WriteFile(userMapPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to create user map file: %v", err)
	}

	cfg := setDefaults(true)
	cfg.Server.Sources = []*Source{{Path: "."}}
	cfg.Server.Filesystem.UnixUserContext.Enabled = true
	cfg.Server.Filesystem.UnixUserContext.HelperPath = helperPath
	cfg.Server.Filesystem.UnixUserContext.UserMapFile = userMapPath
	cfg.Server.Filesystem.UnixUserContext.HelperTimeoutMs = 2500

	Config = cfg
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("expected unix user context config to validate, got error: %v", err)
	}
}

func TestValidateConfig_UnixUserContextEnabledInvalidHelperPath(t *testing.T) {
	cfg := setDefaults(true)
	cfg.Server.Sources = []*Source{{Path: "."}}
	cfg.Server.Filesystem.UnixUserContext.Enabled = true
	cfg.Server.Filesystem.UnixUserContext.HelperPath = "/does/not/exist"
	cfg.Server.Filesystem.UnixUserContext.UserMapFile = "/does/not/exist-map"
	cfg.Server.Filesystem.UnixUserContext.HelperTimeoutMs = 2500

	Config = cfg
	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for invalid unix user context paths")
	}
}

func TestLoadEnvConfig_UnixUserContextOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	helperPath := filepath.Join(tmpDir, "helper")
	userMapPath := filepath.Join(tmpDir, "user-map.json")

	t.Setenv("FILEBROWSER_UNIX_USER_CONTEXT_ENABLED", "true")
	t.Setenv("FILEBROWSER_UNIX_USER_CONTEXT_FALLBACK", "false")
	t.Setenv("FILEBROWSER_UNIX_USER_CONTEXT_HELPER_PATH", helperPath)
	t.Setenv("FILEBROWSER_UNIX_USER_CONTEXT_USER_MAP_FILE", userMapPath)
	t.Setenv("FILEBROWSER_UNIX_USER_CONTEXT_TIMEOUT_MS", "3200")

	Config = setDefaults(true)
	loadEnvConfig()

	ctx := Config.Server.Filesystem.UnixUserContext
	if !ctx.Enabled {
		t.Fatal("expected unix user context enabled via env override")
	}
	if ctx.FallbackToServiceUser {
		t.Fatal("expected fallbackToServiceUser=false via env override")
	}
	if ctx.HelperPath != helperPath {
		t.Fatalf("expected helper path %q, got %q", helperPath, ctx.HelperPath)
	}
	if ctx.UserMapFile != userMapPath {
		t.Fatalf("expected user map file %q, got %q", userMapPath, ctx.UserMapFile)
	}
	if ctx.HelperTimeoutMs != 3200 {
		t.Fatalf("expected helper timeout 3200, got %d", ctx.HelperTimeoutMs)
	}
}
