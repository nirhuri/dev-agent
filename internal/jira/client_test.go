package jira_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nirhuri/dev-agent/internal/jira"
)

func TestBuildJQL(t *testing.T) {
	jql := jira.BuildJQL(jira.ListOptions{
		ProjectKey: "APP",
		Status:     "To Do",
		Assignee:   "me",
		TextSearch: "auth",
	})
	if !strings.Contains(jql, `project = "APP"`) {
		t.Fatalf("jql missing project: %s", jql)
	}
	if !strings.Contains(jql, "assignee = currentUser()") {
		t.Fatalf("jql missing assignee: %s", jql)
	}
	if !strings.Contains(jql, `text ~ "auth"`) {
		t.Fatalf("jql missing text: %s", jql)
	}
}

func TestBuildJQLExplicitOverride(t *testing.T) {
	jql := jira.BuildJQL(jira.ListOptions{JQL: "project = X", ProjectKey: "Y"})
	if jql != "project = X" {
		t.Fatalf("got %q", jql)
	}
}

func TestListIssues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" && r.URL.Path != "/rest/api/3/search" {
			http.NotFound(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Basic ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"startAt":    0,
			"maxResults": 50,
			"total":      1,
			"issues": []map[string]any{
				{
					"id":  "10001",
					"key": "APP-142",
					"fields": map[string]any{
						"summary":   "Add user authentication",
						"status":    map[string]any{"name": "To Do"},
						"issuetype": map[string]any{"name": "Story"},
						"assignee":  map[string]any{"displayName": "Ada"},
						"priority":  map[string]any{"name": "Medium"},
						"updated":   "2026-01-02T15:04:05.000+0000",
					},
				},
			},
		})
	}))
	defer server.Close()

	client, err := jira.NewClient(server.URL, "dev@example.com", "token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.ListIssues(context.Background(), jira.ListOptions{ProjectKey: "APP"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("issues = %d", len(result.Issues))
	}
	issue := result.Issues[0]
	if issue.Key != "APP-142" || issue.Summary != "Add user authentication" {
		t.Fatalf("issue = %+v", issue)
	}
	if issue.Status != "To Do" || issue.Assignee != "Ada" {
		t.Fatalf("mapped fields = %+v", issue)
	}
}

func TestListIssuesUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errorMessages":["auth"]}`))
	}))
	defer server.Close()

	client, err := jira.NewClient(server.URL, "a@b.c", "bad", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ListIssues(context.Background(), jira.ListOptions{})
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !strings.Contains(err.Error(), "authentication") {
		t.Fatalf("error = %v", err)
	}
}

func TestListIssuesRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client, err := jira.NewClient(server.URL, "a@b.c", "tok", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ListIssues(context.Background(), jira.ListOptions{})
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("error = %v", err)
	}
}

func TestGetIssueWithADFDescription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/APP-1") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":  "1",
			"key": "APP-1",
			"fields": map[string]any{
				"summary":   "Test",
				"status":    map[string]any{"name": "Open"},
				"issuetype": map[string]any{"name": "Task"},
				"description": map[string]any{
					"type": "doc",
					"content": []any{
						map[string]any{
							"type": "paragraph",
							"content": []any{
								map[string]any{"type": "text", "text": "Do the thing"},
							},
						},
					},
				},
				"comment": map[string]any{
					"comments": []any{
						map[string]any{
							"id":     "c1",
							"author": map[string]any{"displayName": "Bob"},
							"body": map[string]any{
								"type": "doc",
								"content": []any{
									map[string]any{
										"type": "paragraph",
										"content": []any{
											map[string]any{"type": "text", "text": "Looks good"},
										},
									},
								},
							},
							"created": "2026-01-02T15:04:05.000+0000",
						},
					},
				},
				"labels": []string{"backend"},
			},
		})
	}))
	defer server.Close()

	client, err := jira.NewClient(server.URL, "a@b.c", "tok", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	detail, err := client.GetIssue(context.Background(), "APP-1")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Description != "Do the thing" {
		t.Fatalf("description = %q", detail.Description)
	}
	if len(detail.Comments) != 1 || detail.Comments[0].Body != "Looks good" {
		t.Fatalf("comments = %+v", detail.Comments)
	}
	if len(detail.Labels) != 1 || detail.Labels[0] != "backend" {
		t.Fatalf("labels = %+v", detail.Labels)
	}
}

func TestNewClientValidation(t *testing.T) {
	_, err := jira.NewClient("", "a", "b", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
