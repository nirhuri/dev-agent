package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nirhuri/dev-agent/internal/cli"
	"github.com/nirhuri/dev-agent/internal/config"
)

func TestRootWithoutConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := cli.NewRootCmd(stdout, stderr)
	cmd.SetArgs([]string{"--config", cfgPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Welcome to Dev Agent") {
		t.Fatalf("output missing welcome: %s", out)
	}
	if !strings.Contains(out, "dev-agent setup") {
		t.Fatalf("output should suggest setup: %s", out)
	}
}

func TestRootWithConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := config.Default()
	cfg.Jira.BaseURL = "https://example.atlassian.net"
	cfg.Jira.Email = "dev@example.com"
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}

	stdout := &bytes.Buffer{}
	cmd := cli.NewRootCmd(stdout, &bytes.Buffer{})
	cmd.SetArgs([]string{"--config", cfgPath})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if !strings.Contains(out, "https://example.atlassian.net") {
		t.Fatalf("expected jira url in output: %s", out)
	}
}

func TestConfigShowRedactsSecrets(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(config.Default(), cfgPath); err != nil {
		t.Fatal(err)
	}

	stdout := &bytes.Buffer{}
	cmd := cli.NewRootCmd(stdout, &bytes.Buffer{})
	cmd.SetArgs([]string{"--config", cfgPath, "config", "show"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if strings.Contains(out, "api_token_value") {
		t.Fatal("should not leak secret values")
	}
	if !strings.Contains(out, "keyring") {
		t.Fatalf("expected keyring placeholder: %s", out)
	}
}

func TestConfigSetAndShow(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(config.Default(), cfgPath); err != nil {
		t.Fatal(err)
	}

	stdout := &bytes.Buffer{}
	cmd := cli.NewRootCmd(stdout, &bytes.Buffer{})
	cmd.SetArgs([]string{"--config", cfgPath, "config", "set", "mode", "autonomous"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	loaded, _, err := config.Load(config.LoadOptions{ConfigFile: cfgPath})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Mode != "autonomous" {
		t.Fatalf("mode = %q", loaded.Mode)
	}
}

func TestDoctorRuns(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(config.Default(), cfgPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEV_AGENT_DATA_DIR", filepath.Join(dir, "data"))

	stdout := &bytes.Buffer{}
	cmd := cli.NewRootCmd(stdout, &bytes.Buffer{})
	cmd.SetArgs([]string{"--config", cfgPath, "doctor"})
	// Doctor may return error if git missing etc.; we still check output.
	_ = cmd.Execute()
	out := stdout.String()
	if !strings.Contains(out, "Dev Agent doctor") {
		t.Fatalf("missing doctor header: %s", out)
	}
	if !strings.Contains(out, "config:") {
		t.Fatalf("missing config check: %s", out)
	}
}

func TestSetupNonInteractive(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	repo := filepath.Join(dir, "repo")
	if err := osMkdir(repo); err != nil {
		t.Fatal(err)
	}

	input := strings.Join([]string{
		"interactive", // mode
		"",            // skip jira url
		"cursor",      // engine
		"agent",       // agent path
		"",            // skip api key
		repo,          // repo path
		"testrepo",    // repo name
		"3",           // repair attempts
		"",
	}, "\n")

	stdout := &bytes.Buffer{}
	cmd := cli.NewRootCmd(stdout, &bytes.Buffer{})
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs([]string{"--config", cfgPath, "setup"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("setup: %v\n%s", err, stdout.String())
	}

	loaded, _, err := config.Load(config.LoadOptions{ConfigFile: cfgPath})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Engine.Provider != "cursor" {
		t.Fatalf("provider = %q", loaded.Engine.Provider)
	}
	if len(loaded.Repos) != 1 || loaded.Repos[0].Name != "testrepo" {
		t.Fatalf("repos = %+v", loaded.Repos)
	}
}

func osMkdir(path string) error {
	return os.MkdirAll(path, 0o700)
}
