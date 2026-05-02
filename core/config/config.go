// Package config handles core and app configuration via environment variables.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
)

const sqliteDriver = "sqlite"

// Config holds core and application configuration populated from environment variables.
type Config struct {
	Core CoreConfig
	App  AppConfig
}

// CoreConfig holds stable kumacore runtime configuration.
type CoreConfig struct {
	Server struct {
		Host string `envconfig:"CORE_SERVER_HOST" default:"127.0.0.1"`
		Port int    `envconfig:"CORE_SERVER_PORT" default:"3000"`
	}

	App struct {
		Dev bool `envconfig:"CORE_APP_DEV" default:"true"`
	}

	DB struct {
		Driver string `envconfig:"CORE_DB_DRIVER" default:"sqlite"`
		Path   string `envconfig:"CORE_DB_PATH"   default:"./data/db/kumacore.db"`
	}

	Logging struct {
		Dir string `envconfig:"CORE_LOG_DIR" default:"./data/logs"`
	}

	Session struct {
		TTL time.Duration `envconfig:"CORE_SESSION_TTL" default:"168h"`
	}

	Worker struct {
		Enabled      bool          `envconfig:"CORE_WORKER_ENABLED"       default:"false"`
		PollInterval time.Duration `envconfig:"CORE_WORKER_POLL_INTERVAL" default:"5s"`
		MaxAttempts  int           `envconfig:"CORE_WORKER_MAX_ATTEMPTS"  default:"3"`
	}
}

// AppConfig holds app-owned runtime configuration.
type AppConfig struct {
	Name    string `envconfig:"APP_NAME"     default:"kumacore"`
	RootDir string `envconfig:"APP_ROOT_DIR" default:"."`
}

// Load reads environment variables into a Config using envconfig.
func Load() (*Config, error) {
	var configuration Config
	if err := envconfig.Process("", &configuration); err != nil {
		return nil, fmt.Errorf("[config:Load] load env: %w", err)
	}

	if err := configuration.Validate(); err != nil {
		return nil, err
	}

	return &configuration, nil
}

// Validate fails fast on invalid critical configuration.
func (configuration Config) Validate() error {
	if strings.TrimSpace(configuration.Core.Server.Host) == "" {
		return fmt.Errorf("[config:Validate] CORE_SERVER_HOST is required")
	}

	if configuration.Core.Server.Port < 1 || configuration.Core.Server.Port > 65535 {
		return fmt.Errorf("[config:Validate] CORE_SERVER_PORT must be between 1 and 65535")
	}

	if configuration.Core.DB.Driver != sqliteDriver {
		return fmt.Errorf("[config:Validate] unsupported CORE_DB_DRIVER %q", configuration.Core.DB.Driver)
	}

	if strings.TrimSpace(configuration.Core.DB.Path) == "" {
		return fmt.Errorf("[config:Validate] CORE_DB_PATH is required")
	}

	if strings.TrimSpace(configuration.Core.Logging.Dir) == "" {
		return fmt.Errorf("[config:Validate] CORE_LOG_DIR is required")
	}

	if configuration.Core.Session.TTL <= 0 {
		return fmt.Errorf("[config:Validate] CORE_SESSION_TTL must be positive")
	}

	if configuration.Core.Worker.Enabled {
		if configuration.Core.Worker.PollInterval <= 0 {
			return fmt.Errorf("[config:Validate] CORE_WORKER_POLL_INTERVAL must be positive")
		}

		if configuration.Core.Worker.MaxAttempts < 1 {
			return fmt.Errorf("[config:Validate] CORE_WORKER_MAX_ATTEMPTS must be at least 1")
		}
	}

	if strings.TrimSpace(configuration.App.Name) == "" {
		return fmt.Errorf("[config:Validate] APP_NAME is required")
	}

	if strings.TrimSpace(configuration.App.RootDir) == "" {
		return fmt.Errorf("[config:Validate] APP_ROOT_DIR is required")
	}

	return nil
}
