package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

// LoadOptions controls how configuration is loaded.
type LoadOptions struct {
	// ConfigFile overrides the default config path. Empty uses FilePath().
	ConfigFile string
	// EnvPrefix defaults to DEV_AGENT.
	EnvPrefix string
}

// Load reads configuration from file, environment, and defaults.
// Precedence (highest wins): environment variables > config file > defaults.
func Load(opts LoadOptions) (Config, string, error) {
	cfg := Default()

	path := opts.ConfigFile
	if path == "" {
		var err error
		path, err = FilePath()
		if err != nil {
			return cfg, "", err
		}
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	prefix := opts.EnvPrefix
	if prefix == "" {
		prefix = "DEV_AGENT"
	}
	v.SetEnvPrefix(prefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	bindEnv(v, "mode")
	bindEnv(v, "jira.base_url")
	bindEnv(v, "jira.email")
	bindEnv(v, "jira.project_key")
	bindEnv(v, "jira.jql")
	bindEnv(v, "jira.max_results")
	bindEnv(v, "engine.provider")
	bindEnv(v, "engine.agent_path")
	bindEnv(v, "engine.model")
	bindEnv(v, "git.pr_provider")
	bindEnv(v, "validation.repair_attempts")

	source := "defaults"
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			if !os.IsNotExist(err) {
				return cfg, "", fmt.Errorf("read config %s: %w", path, err)
			}
		}
	} else {
		source = path
	}

	if err := v.Unmarshal(&cfg); err != nil {
		return cfg, source, fmt.Errorf("parse config: %w", err)
	}

	// Ensure defaults for zero values that unmarshal may clear.
	if cfg.Version == 0 {
		cfg.Version = Version
	}
	if cfg.Mode == "" {
		cfg.Mode = DefaultMode
	}
	if cfg.Jira.MaxResults == 0 {
		cfg.Jira.MaxResults = DefaultJiraMaxResults
	}
	if cfg.Engine.Provider == "" {
		cfg.Engine.Provider = DefaultEngine
	}
	if cfg.Engine.AgentPath == "" {
		cfg.Engine.AgentPath = "agent"
	}
	if cfg.Git.PRProvider == "" {
		cfg.Git.PRProvider = DefaultPRProvider
	}
	if len(cfg.Git.ProtectedBranches) == 0 {
		cfg.Git.ProtectedBranches = []string{"main", "master"}
	}
	if cfg.Git.BranchPrefix == "" {
		cfg.Git.BranchPrefix = "agent/"
	}
	if cfg.Validation.RepairAttempts == 0 && !v.IsSet("validation.repair_attempts") {
		cfg.Validation.RepairAttempts = DefaultRepairAttempts
	}

	if err := cfg.Validate(); err != nil {
		return cfg, source, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, source, nil
}

func bindEnv(v *viper.Viper, key string) {
	_ = v.BindEnv(key)
}

// Save writes cfg to the given path (or default FilePath) with restrictive permissions.
func Save(cfg Config, path string) error {
	if path == "" {
		var err error
		path, err = FilePath()
		if err != nil {
			return err
		}
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

// Exists reports whether a config file is present at the default or given path.
func Exists(path string) (bool, error) {
	if path == "" {
		var err error
		path, err = FilePath()
		if err != nil {
			return false, err
		}
	}
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
