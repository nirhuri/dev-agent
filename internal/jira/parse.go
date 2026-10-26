package jira

import (
	"encoding/json"
	"strings"
	"time"
)

type searchResponse struct {
	StartAt       int             `json:"startAt"`
	MaxResults    int             `json:"maxResults"`
	Total         int             `json:"total"`
	IsLast        bool            `json:"isLast"`
	NextPageToken string          `json:"nextPageToken"`
	Issues        []issueResponse `json:"issues"`
}

type issueResponse struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	Self   string `json:"self"`
	Fields struct {
		Summary     string          `json:"summary"`
		Status      *namedEntity    `json:"status"`
		IssueType   *namedEntity    `json:"issuetype"`
		Assignee    *userEntity     `json:"assignee"`
		Priority    *namedEntity    `json:"priority"`
		Updated     string          `json:"updated"`
		Description json.RawMessage `json:"description"`
		Labels      []string        `json:"labels"`
		Components  []namedEntity   `json:"components"`
		Parent      *struct {
			Key string `json:"key"`
		} `json:"parent"`
		IssueLinks []issueLink `json:"issuelinks"`
		Comment    *struct {
			Comments []commentRaw `json:"comments"`
		} `json:"comment"`
		CustomField10016 json.RawMessage `json:"customfield_10016"`
		CustomField10028 json.RawMessage `json:"customfield_10028"`
	} `json:"fields"`
}

type namedEntity struct {
	Name string `json:"name"`
}

type userEntity struct {
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
	AccountID    string `json:"accountId"`
}

type issueLink struct {
	OutwardIssue *struct {
		Key string `json:"key"`
	} `json:"outwardIssue"`
	InwardIssue *struct {
		Key string `json:"key"`
	} `json:"inwardIssue"`
}

type commentRaw struct {
	ID      string          `json:"id"`
	Author  *userEntity     `json:"author"`
	Body    json.RawMessage `json:"body"`
	Created string          `json:"created"`
}

func mapIssue(raw issueResponse) Issue {
	issue := Issue{
		Key:     raw.Key,
		ID:      raw.ID,
		SelfURL: raw.Self,
		Summary: raw.Fields.Summary,
		Updated: parseTime(raw.Fields.Updated),
	}
	if raw.Fields.Status != nil {
		issue.Status = raw.Fields.Status.Name
	}
	if raw.Fields.IssueType != nil {
		issue.IssueType = raw.Fields.IssueType.Name
	}
	if raw.Fields.Assignee != nil {
		issue.Assignee = displayName(raw.Fields.Assignee)
	}
	if raw.Fields.Priority != nil {
		issue.Priority = raw.Fields.Priority.Name
	}
	return issue
}

func displayName(u *userEntity) string {
	if u == nil {
		return ""
	}
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.EmailAddress
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05.000+0000",
		"2006-01-02T15:04:05-0700",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// extractADFText flattens Atlassian Document Format (or plain string) to text.
func extractADFText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return strings.TrimSpace(string(raw))
	}
	var b strings.Builder
	walkADF(doc, &b)
	return strings.TrimSpace(b.String())
}

func walkADF(node any, b *strings.Builder) {
	switch n := node.(type) {
	case map[string]any:
		if t, ok := n["type"].(string); ok && t == "text" {
			if text, ok := n["text"].(string); ok {
				b.WriteString(text)
			}
		}
		if content, ok := n["content"].([]any); ok {
			for i, child := range content {
				if i > 0 {
					if ct, _ := n["type"].(string); ct == "doc" || ct == "paragraph" || ct == "heading" || ct == "listItem" {
						if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
							b.WriteByte('\n')
						}
					}
				}
				walkADF(child, b)
			}
		}
	case []any:
		for _, child := range n {
			walkADF(child, b)
		}
	}
}
