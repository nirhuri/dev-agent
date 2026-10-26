package cli_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nirhuri/dev-agent/internal/cli"
	"github.com/nirhuri/dev-agent/internal/config"
)

func TestIssuesSelectWithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/APP-99") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":  "99",
			"key": "APP-99",
			"fields": map[string]any{
				"summary":     "Mock issue",
				"status":      map[string]any{"name": "To Do"},
				"issuetype":   map[string]any{"name": "Bug"},
				"description": "Fix it",
				"comment":     map[string]any{"comments": []any{}},
			},
		})
	}))
	defer server.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := config.Default()
	cfg.Jira.BaseURL = server.URL
	cfg.Jira.Email = "dev@example.com"
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEV_AGENT_JIRA_TOKEN", "test-token")

	stdout := &bytes.Buffer{}
	cmd := cli.NewRootCmd(stdout, &bytes.Buffer{})
	cmd.SetArgs([]string{"--config", cfgPath, "issues", "--select", "APP-99"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "APP-99") || !strings.Contains(out, "Mock issue") {
		t.Fatalf("output = %s", out)
	}
}

func TestIssuesRequiresConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(config.Default(), cfgPath); err != nil {
		t.Fatal(err)
	}
	stdout := &bytes.Buffer{}
	cmd := cli.NewRootCmd(stdout, &bytes.Buffer{})
	cmd.SetArgs([]string{"--config", cfgPath, "issues"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when jira not configured")
	}
}
