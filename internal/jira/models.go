package jira

import "time"

// Issue is a summarized Jira issue for list views.
type Issue struct {
	Key       string `json:"key"`
	ID        string `json:"id"`
	Summary   string `json:"summary"`
	Status    string `json:"status"`
	IssueType string `json:"issue_type"`
	Assignee  string `json:"assignee"`
	Priority  string `json:"priority"`
	Updated   time.Time
	SelfURL   string `json:"self"`
}

// IssueDetail is a full issue including description and comments.
type IssueDetail struct {
	Issue
	Description string
	Acceptance  string
	Comments    []Comment
	Labels      []string
	Components  []string
	ParentKey   string
	LinkedKeys  []string
	RawFields   map[string]any
}

// Comment is a Jira issue comment.
type Comment struct {
	ID      string
	Author  string
	Body    string
	Created time.Time
}

// SearchResult is a page of issues from the search API.
type SearchResult struct {
	Issues        []Issue
	StartAt       int
	MaxResults    int
	Total         int
	IsLast        bool
	NextPageToken string
}

// ListOptions controls issue listing.
type ListOptions struct {
	JQL        string
	ProjectKey string
	Status     string
	Assignee   string
	TextSearch string
	StartAt    int
	MaxResults int
}
