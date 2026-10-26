package jira

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client talks to Jira Cloud REST API v3.
type Client struct {
	baseURL    string
	email      string
	token      string
	httpClient *http.Client
}

// NewClient creates a Jira Cloud API client.
func NewClient(baseURL, email, token string, httpClient *http.Client) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("jira base URL is required")
	}
	if email == "" {
		return nil, fmt.Errorf("jira email is required")
	}
	if token == "" {
		return nil, fmt.Errorf("jira API token is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		baseURL:    baseURL,
		email:      email,
		token:      token,
		httpClient: httpClient,
	}, nil
}

// BuildJQL constructs a JQL string from list options.
func BuildJQL(opts ListOptions) string {
	if strings.TrimSpace(opts.JQL) != "" {
		return opts.JQL
	}
	var parts []string
	if opts.ProjectKey != "" {
		parts = append(parts, fmt.Sprintf(`project = "%s"`, escapeJQL(opts.ProjectKey)))
	}
	if opts.Status != "" {
		parts = append(parts, fmt.Sprintf(`status = "%s"`, escapeJQL(opts.Status)))
	}
	if opts.Assignee != "" {
		if strings.EqualFold(opts.Assignee, "me") || strings.EqualFold(opts.Assignee, "currentUser()") {
			parts = append(parts, "assignee = currentUser()")
		} else {
			parts = append(parts, fmt.Sprintf(`assignee = "%s"`, escapeJQL(opts.Assignee)))
		}
	}
	if opts.TextSearch != "" {
		parts = append(parts, fmt.Sprintf(`text ~ "%s"`, escapeJQL(opts.TextSearch)))
	}
	if len(parts) == 0 {
		return "order by updated DESC"
	}
	return strings.Join(parts, " AND ") + " ORDER BY updated DESC"
}

func escapeJQL(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

// ListIssues searches for issues using JQL.
func (c *Client) ListIssues(ctx context.Context, opts ListOptions) (*SearchResult, error) {
	jql := BuildJQL(opts)
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 50
	}
	if maxResults > 100 {
		maxResults = 100
	}

	q := url.Values{}
	q.Set("jql", jql)
	q.Set("startAt", strconv.Itoa(opts.StartAt))
	q.Set("maxResults", strconv.Itoa(maxResults))
	q.Set("fields", "summary,status,issuetype,assignee,priority,updated")

	var raw searchResponse
	if err := c.doJSON(ctx, http.MethodGet, "/rest/api/3/search/jql?"+q.Encode(), nil, &raw); err != nil {
		// Fall back to classic search endpoint for compatibility.
		if isNotFound(err) {
			q2 := url.Values{}
			q2.Set("jql", jql)
			q2.Set("startAt", strconv.Itoa(opts.StartAt))
			q2.Set("maxResults", strconv.Itoa(maxResults))
			q2.Set("fields", "summary,status,issuetype,assignee,priority,updated")
			if err2 := c.doJSON(ctx, http.MethodGet, "/rest/api/3/search?"+q2.Encode(), nil, &raw); err2 != nil {
				return nil, err2
			}
		} else {
			return nil, err
		}
	}

	result := &SearchResult{
		StartAt:       raw.StartAt,
		MaxResults:    raw.MaxResults,
		Total:         raw.Total,
		IsLast:        raw.IsLast,
		NextPageToken: raw.NextPageToken,
	}
	for _, item := range raw.Issues {
		result.Issues = append(result.Issues, mapIssue(item))
	}
	if result.Total == 0 && len(result.Issues) > 0 {
		result.Total = len(result.Issues)
	}
	if !result.IsLast && result.Total > 0 {
		result.IsLast = result.StartAt+len(result.Issues) >= result.Total
	}
	return result, nil
}

// GetIssue fetches full issue details including comments.
func (c *Client) GetIssue(ctx context.Context, key string) (*IssueDetail, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("issue key is required")
	}

	path := "/rest/api/3/issue/" + url.PathEscape(key) +
		"?fields=summary,status,issuetype,assignee,priority,updated,description,comment,labels,components,parent,issuelinks,customfield_10016,customfield_10028" +
		"&expand=renderedFields"

	var raw issueResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &raw); err != nil {
		return nil, err
	}

	detail := &IssueDetail{
		Issue:     mapIssue(raw),
		Labels:    raw.Fields.Labels,
		RawFields: map[string]any{},
	}
	detail.Description = extractADFText(raw.Fields.Description)
	if raw.Fields.Parent != nil {
		detail.ParentKey = raw.Fields.Parent.Key
	}
	for _, cpt := range raw.Fields.Components {
		detail.Components = append(detail.Components, cpt.Name)
	}
	for _, link := range raw.Fields.IssueLinks {
		if link.OutwardIssue != nil {
			detail.LinkedKeys = append(detail.LinkedKeys, link.OutwardIssue.Key)
		}
		if link.InwardIssue != nil {
			detail.LinkedKeys = append(detail.LinkedKeys, link.InwardIssue.Key)
		}
	}
	if raw.Fields.Comment != nil {
		for _, cmt := range raw.Fields.Comment.Comments {
			detail.Comments = append(detail.Comments, Comment{
				ID:      cmt.ID,
				Author:  displayName(cmt.Author),
				Body:    extractADFText(cmt.Body),
				Created: parseTime(cmt.Created),
			})
		}
	}
	// Common acceptance-criteria custom fields; keep empty if absent.
	detail.Acceptance = firstNonEmpty(
		extractADFText(raw.Fields.CustomField10016),
		extractADFText(raw.Fields.CustomField10028),
	)
	return detail, nil
}

// Ping verifies credentials with a lightweight myself call.
func (c *Client) Ping(ctx context.Context) error {
	var raw map[string]any
	return c.doJSON(ctx, http.MethodGet, "/rest/api/3/myself", nil, &raw)
}

func (c *Client) doJSON(ctx context.Context, method, path string, body io.Reader, dest any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	auth := base64.StdEncoding.EncodeToString([]byte(c.email + ":" + c.token))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("jira request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("read jira response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &APIError{Status: resp.StatusCode, Message: "authentication failed; check email and API token"}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		retry := resp.Header.Get("Retry-After")
		msg := "rate limited by Jira"
		if retry != "" {
			msg += "; retry after " + retry + "s"
		}
		return &APIError{Status: resp.StatusCode, Message: msg}
	}
	if resp.StatusCode == http.StatusNotFound {
		return &APIError{Status: resp.StatusCode, Message: "resource not found"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{Status: resp.StatusCode, Message: truncate(string(data), 200)}
	}
	if dest == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("decode jira response: %w", err)
	}
	return nil
}

// APIError is a Jira HTTP API failure.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("jira API error (%d): %s", e.Status, e.Message)
}

func isNotFound(err error) bool {
	var api *APIError
	if ok := errorAs(err, &api); ok {
		return api.Status == http.StatusNotFound
	}
	return false
}

func errorAs(err error, target **APIError) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*APIError); ok {
		*target = e
		return true
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
