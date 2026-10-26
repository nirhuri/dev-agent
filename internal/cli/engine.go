package cli

import (
	"fmt"
	"os"

	"github.com/nirhuri/dev-agent/internal/config"
	"github.com/nirhuri/dev-agent/internal/engine"
	"github.com/nirhuri/dev-agent/internal/engine/cursor"
	"github.com/nirhuri/dev-agent/internal/secrets"
)

func newEngine(cfg config.Config) (engine.Engine, error) {
	switch cfg.Engine.Provider {
	case "", "cursor":
		apiKey := os.Getenv("CURSOR_API_KEY")
		if apiKey == "" {
			if v, err := secrets.Get(secrets.KeyCursorAPIKey); err == nil {
				apiKey = v
			}
		}
		return cursor.New(cfg.Engine.AgentPath, apiKey, cfg.Engine.Model), nil
	case "fake":
		return &engine.Fake{}, nil
	default:
		return nil, fmt.Errorf("unknown engine provider %q", cfg.Engine.Provider)
	}
}
