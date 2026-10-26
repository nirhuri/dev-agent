package secrets

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const service = "dev-agent"

// Known secret keys stored in the OS credential store.
const (
	KeyJiraToken    = "jira_token"
	KeyCursorAPIKey = "cursor_api_key"
)

var (
	// ErrNotFound is returned when a secret is not present.
	ErrNotFound = errors.New("secret not found")
)

// Store persists a secret in the OS keyring.
func Store(key, value string) error {
	if key == "" {
		return fmt.Errorf("secret key is required")
	}
	if value == "" {
		return fmt.Errorf("secret value is required")
	}
	if err := keyring.Set(service, key, value); err != nil {
		return fmt.Errorf("store secret %q: %w", key, err)
	}
	return nil
}

// Get retrieves a secret from the OS keyring.
func Get(key string) (string, error) {
	value, err := keyring.Get(service, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("get secret %q: %w", key, err)
	}
	return value, nil
}

// Delete removes a secret from the OS keyring.
func Delete(key string) error {
	err := keyring.Delete(service, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("delete secret %q: %w", key, err)
	}
	return nil
}

// Exists reports whether a secret is present without returning its value.
func Exists(key string) bool {
	_, err := Get(key)
	return err == nil
}

// Status returns a human-readable presence indicator (never the secret value).
func Status(key string) string {
	if Exists(key) {
		return "set (keyring)"
	}
	return "not set"
}
