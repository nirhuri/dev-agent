package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/nirhuri/dev-agent/internal/workflow"
)

// SQLite is a Store backed by a pure-Go SQLite database (no CGO).
type SQLite struct {
	db *sql.DB
}

// OpenSQLite opens (or creates) the task database at path.
func OpenSQLite(path string) (*SQLite, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pragma: %w", err)
	}
	s := &SQLite{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLite) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  issue_key TEXT NOT NULL,
  issue_summary TEXT NOT NULL DEFAULT '',
  repository TEXT NOT NULL DEFAULT '',
  branch TEXT NOT NULL DEFAULT '',
  engine TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL,
  plan TEXT NOT NULL DEFAULT '',
  clarifications_json TEXT NOT NULL DEFAULT '[]',
  validation_json TEXT NOT NULL DEFAULT '[]',
  commit_hashes_json TEXT NOT NULL DEFAULT '[]',
  pr_url TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  repair_attempt INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tasks_issue ON tasks(issue_key);
CREATE INDEX IF NOT EXISTS idx_tasks_state ON tasks(state);

CREATE TABLE IF NOT EXISTS transitions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  from_state TEXT NOT NULL,
  to_state TEXT NOT NULL,
  note TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS locks (
  task_id TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
  owner TEXT NOT NULL,
  expires_at TEXT NOT NULL
);
`
	_, err := s.db.Exec(schema)
	return err
}

func (s *SQLite) Close() error {
	return s.db.Close()
}

func (s *SQLite) CreateTask(ctx context.Context, task *workflow.Task) error {
	if task.ID == "" {
		task.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	task.CreatedAt = now
	task.UpdatedAt = now
	if task.State == "" {
		task.State = workflow.StateIdle
	}
	clar, _ := json.Marshal(task.Clarifications)
	valid, _ := json.Marshal(task.Validation)
	commits, _ := json.Marshal(task.CommitHashes)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO tasks (
  id, issue_key, issue_summary, repository, branch, engine, state, plan,
  clarifications_json, validation_json, commit_hashes_json, pr_url, error_message,
  repair_attempt, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.IssueKey, task.IssueSummary, task.Repository, task.Branch, task.Engine, string(task.State), task.Plan,
		string(clar), string(valid), string(commits), task.PRURL, task.ErrorMessage,
		task.RepairAttempt, task.CreatedAt.Format(time.RFC3339Nano), task.UpdatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("create task: %w", err)
	}
	_, _ = s.db.ExecContext(ctx, `
INSERT INTO transitions (task_id, from_state, to_state, note, created_at)
VALUES (?, ?, ?, ?, ?)`, task.ID, "", string(task.State), "created", task.CreatedAt.Format(time.RFC3339Nano))
	return nil
}

func (s *SQLite) GetTask(ctx context.Context, id string) (*workflow.Task, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id)
	return scanTask(row)
}

func (s *SQLite) GetTaskByIssue(ctx context.Context, issueKey string) (*workflow.Task, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT `+taskColumns+` FROM tasks
WHERE issue_key = ? AND state NOT IN ('COMPLETED', 'CANCELLED')
ORDER BY updated_at DESC LIMIT 1`, issueKey)
	return scanTask(row)
}

func (s *SQLite) ListTasks(ctx context.Context, includeTerminal bool) ([]workflow.Task, error) {
	q := `SELECT ` + taskColumns + ` FROM tasks`
	if !includeTerminal {
		q += ` WHERE state NOT IN ('COMPLETED', 'CANCELLED')`
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []workflow.Task
	for rows.Next() {
		t, err := scanTaskRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (s *SQLite) UpdateTask(ctx context.Context, task *workflow.Task) error {
	task.UpdatedAt = time.Now().UTC()
	clar, _ := json.Marshal(task.Clarifications)
	valid, _ := json.Marshal(task.Validation)
	commits, _ := json.Marshal(task.CommitHashes)
	// State changes only via Transition to keep the audit trail authoritative.
	res, err := s.db.ExecContext(ctx, `
UPDATE tasks SET
  issue_summary=?, repository=?, branch=?, engine=?, plan=?,
  clarifications_json=?, validation_json=?, commit_hashes_json=?, pr_url=?,
  error_message=?, repair_attempt=?, updated_at=?
WHERE id=?`,
		task.IssueSummary, task.Repository, task.Branch, task.Engine, task.Plan,
		string(clar), string(valid), string(commits), task.PRURL,
		task.ErrorMessage, task.RepairAttempt, task.UpdatedAt.Format(time.RFC3339Nano), task.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return workflow.ErrNotFound
	}
	return nil
}

func (s *SQLite) Transition(ctx context.Context, taskID string, to workflow.State, note string) (*workflow.Task, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, taskID)
	task, err := scanTask(row)
	if err != nil {
		return nil, err
	}
	if !workflow.CanTransition(task.State, to) {
		return nil, fmt.Errorf("%w: %s -> %s", workflow.ErrInvalidTransition, task.State, to)
	}
	from := task.State
	task.State = to
	task.UpdatedAt = time.Now().UTC()
	if to != workflow.StateFailed {
		task.ErrorMessage = ""
	}
	clar, _ := json.Marshal(task.Clarifications)
	valid, _ := json.Marshal(task.Validation)
	commits, _ := json.Marshal(task.CommitHashes)
	if _, err := tx.ExecContext(ctx, `
UPDATE tasks SET state=?, clarifications_json=?, validation_json=?, commit_hashes_json=?,
  error_message=?, updated_at=? WHERE id=?`,
		string(task.State), string(clar), string(valid), string(commits),
		task.ErrorMessage, task.UpdatedAt.Format(time.RFC3339Nano), task.ID,
	); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO transitions (task_id, from_state, to_state, note, created_at)
VALUES (?, ?, ?, ?, ?)`, task.ID, string(from), string(to), note, task.UpdatedAt.Format(time.RFC3339Nano)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *SQLite) ListTransitions(ctx context.Context, taskID string) ([]workflow.TransitionEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, task_id, from_state, to_state, note, created_at
FROM transitions WHERE task_id = ? ORDER BY id ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []workflow.TransitionEvent
	for rows.Next() {
		var e workflow.TransitionEvent
		var from, to, created string
		if err := rows.Scan(&e.ID, &e.TaskID, &from, &to, &e.Note, &created); err != nil {
			return nil, err
		}
		e.FromState = workflow.State(from)
		e.ToState = workflow.State(to)
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLite) AcquireLock(ctx context.Context, taskID, owner string, ttl time.Duration) error {
	now := time.Now().UTC()
	expires := now.Add(ttl)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var currentOwner, expiresAt string
	err = tx.QueryRowContext(ctx, `SELECT owner, expires_at FROM locks WHERE task_id = ?`, taskID).Scan(&currentOwner, &expiresAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		exp, _ := time.Parse(time.RFC3339Nano, expiresAt)
		if exp.After(now) && currentOwner != owner {
			return fmt.Errorf("%w: task locked by %s until %s", workflow.ErrConflict, currentOwner, expiresAt)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM locks WHERE task_id = ?`, taskID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO locks (task_id, owner, expires_at) VALUES (?, ?, ?)`,
		taskID, owner, expires.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLite) ReleaseLock(ctx context.Context, taskID, owner string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM locks WHERE task_id = ? AND owner = ?`, taskID, owner)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return workflow.ErrConflict
	}
	return nil
}

const taskColumns = `id, issue_key, issue_summary, repository, branch, engine, state, plan,
 clarifications_json, validation_json, commit_hashes_json, pr_url, error_message,
 repair_attempt, created_at, updated_at`

type scanner interface {
	Scan(dest ...any) error
}

func scanTask(row scanner) (*workflow.Task, error) {
	var t workflow.Task
	var state, clar, valid, commits, created, updated string
	err := row.Scan(
		&t.ID, &t.IssueKey, &t.IssueSummary, &t.Repository, &t.Branch, &t.Engine, &state, &t.Plan,
		&clar, &valid, &commits, &t.PRURL, &t.ErrorMessage, &t.RepairAttempt, &created, &updated,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, workflow.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.State = workflow.State(state)
	_ = json.Unmarshal([]byte(clar), &t.Clarifications)
	_ = json.Unmarshal([]byte(valid), &t.Validation)
	_ = json.Unmarshal([]byte(commits), &t.CommitHashes)
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	t.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if t.Clarifications == nil {
		t.Clarifications = []workflow.ClarificationQA{}
	}
	if t.Validation == nil {
		t.Validation = []workflow.ValidationResult{}
	}
	if t.CommitHashes == nil {
		t.CommitHashes = []string{}
	}
	return &t, nil
}

func scanTaskRows(rows *sql.Rows) (*workflow.Task, error) {
	return scanTask(rows)
}

// DefaultDBPath returns ~/.local/share/dev-agent/tasks.db (or DEV_AGENT_DATA_DIR).
func DefaultDBPath() (string, error) {
	if override := os.Getenv("DEV_AGENT_DATA_DIR"); override != "" {
		return filepath.Join(override, "tasks.db"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "dev-agent", "tasks.db"), nil
}

// OpenDefault opens the default task database.
func OpenDefault() (*SQLite, error) {
	path, err := DefaultDBPath()
	if err != nil {
		return nil, err
	}
	return OpenSQLite(path)
}
