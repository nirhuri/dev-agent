package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nirhuri/dev-agent/internal/config"
)

func TestDefaultValidate(t *testing.T) {
	cfg := config.Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
}

func TestValidateRejectsBadMode(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = "yolo"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestValidateRejectsBadURL(t *testing.T) {
	cfg := config.Default()
	cfg.Jira.BaseURL = "not-a-url"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for invalid jira.base_url")
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := config.Default()
	cfg.Mode = "autonomous"
	cfg.Jira.BaseURL = "https://example.atlassian.net"
	cfg.Jira.Email = "dev@example.com"
	cfg.Jira.ProjectKey = "APP"
	cfg.Repos = []config.RepoConfig{{Name: "demo", Path: "/tmp/demo"}}

	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("config file permissions too open: %v", info.Mode().Perm())
	}

	loaded, source, err := config.Load(config.LoadOptions{ConfigFile: path})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if source != path {
		t.Fatalf("source = %q, want %q", source, path)
	}
	if loaded.Mode != "autonomous" {
		t.Fatalf("mode = %q", loaded.Mode)
	}
	if loaded.Jira.Email != "dev@example.com" {
		t.Fatalf("email = %q", loaded.Jira.Email)
	}
	if len(loaded.Repos) != 1 || loaded.Repos[0].Name != "demo" {
		t.Fatalf("repos = %+v", loaded.Repos)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := config.Default()
	cfg.Mode = "interactive"
	if err := config.Save(cfg, path); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DEV_AGENT_MODE", "autonomous")
	loaded, _, err := config.Load(config.LoadOptions{ConfigFile: path})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Mode != "autonomous" {
		t.Fatalf("env should override file: got %q", loaded.Mode)
	}
}

func TestRedactedNeverIncludesRawSecrets(t *testing.T) {
	cfg := config.Default()
	cfg.Jira.Email = "secret-looking@example.com"
	redacted := cfg.Redacted()
	jira := redacted["jira"].(map[string]any)
	if jira["token"] != "[stored in keyring]" {
		t.Fatalf("token field = %v", jira["token"])
	}
	engine := redacted["engine"].(map[string]any)
	if engine["api_key"] != "[stored in keyring or env]" {
		t.Fatalf("api_key field = %v", engine["api_key"])
	}
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")
	cfg, source, err := config.Load(config.LoadOptions{ConfigFile: path})
	if err != nil {
		t.Fatal(err)
	}
	if source != "defaults" {
		t.Fatalf("source = %q", source)
	}
	if cfg.Mode != config.DefaultMode {
		t.Fatalf("mode = %q", cfg.Mode)
	}
}

func TestJiraConfigured(t *testing.T) {
	cfg := config.Default()
	if cfg.JiraConfigured() {
		t.Fatal("expected false")
	}
	cfg.Jira.BaseURL = "https://x.atlassian.net"
	cfg.Jira.Email = "a@b.c"
	if !cfg.JiraConfigured() {
		t.Fatal("expected true")
	}
}
