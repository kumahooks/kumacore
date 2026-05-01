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
	Core CoreConfig `envconfig:"CORE"`
	App  AppConfig  `envconfig:"APP"`
}

// CoreConfig holds stable kumacore runtime configuration.
type CoreConfig struct {
	Server struct {
		Host string `envconfig:"HOST" default:"127.0.0.1"`
		Port int    `envconfig:"PORT" default:"3000"`
	}

	App struct {
		Dev bool `envconfig:"DEV" default:"true"`
	}

	DB struct {
		Driver string `envconfig:"DRIVER" default:"sqlite"`
		Path   string `envconfig:"PATH"   default:"./data/kumacore.db"`
	}

	Logging struct {
		Dir string `envconfig:"DIR" default:"./logs"`
	} `envconfig:"LOG"`

	Session struct {
		TTL time.Duration `envconfig:"TTL" default:"168h"`
	}

	Worker struct {
		Enabled      bool          `envconfig:"ENABLED"       default:"false"`
		PollInterval time.Duration `envconfig:"POLL_INTERVAL" default:"5s"`
		MaxAttempts  int           `envconfig:"MAX_ATTEMPTS"  default:"3"`
	}
}

// AppConfig holds app-owned runtime configuration.
type AppConfig struct {
	Name string `envconfig:"NAME"`
}

// Load reads environment variables into a Config using envconfig.
func Load() (*Config, error) {
	var configuration Config
	if err := envconfig.Process("", &configuration); err != nil {
		return nil, fmt.Errorf("[config] load env: %w", err)
	}

	if err := configuration.Validate(); err != nil {
		return nil, err
	}

	return &configuration, nil
}

// Validate fails fast on invalid critical configuration.
func (configuration Config) Validate() error {
	if strings.TrimSpace(configuration.Core.Server.Host) == "" {
		return fmt.Errorf("[config] validate: CORE_SERVER_HOST is required")
	}

	if configuration.Core.Server.Port < 1 || configuration.Core.Server.Port > 65535 {
		return fmt.Errorf("[config] validate: CORE_SERVER_PORT must be between 1 and 65535")
	}

	if configuration.Core.DB.Driver != sqliteDriver {
		return fmt.Errorf("[config] validate: unsupported CORE_DB_DRIVER %q", configuration.Core.DB.Driver)
	}

	if strings.TrimSpace(configuration.Core.DB.Path) == "" {
		return fmt.Errorf("[config] validate: CORE_DB_PATH is required")
	}

	if strings.TrimSpace(configuration.Core.Logging.Dir) == "" {
		return fmt.Errorf("[config] validate: CORE_LOG_DIR is required")
	}

	if configuration.Core.Session.TTL <= 0 {
		return fmt.Errorf("[config] validate: CORE_SESSION_TTL must be positive")
	}

	if configuration.Core.Worker.Enabled {
		if configuration.Core.Worker.PollInterval <= 0 {
			return fmt.Errorf("[config] validate: CORE_WORKER_POLL_INTERVAL must be positive")
		}

		if configuration.Core.Worker.MaxAttempts < 1 {
			return fmt.Errorf("[config] validate: CORE_WORKER_MAX_ATTEMPTS must be at least 1")
		}
	}

	if strings.TrimSpace(configuration.App.Name) == "" {
		return fmt.Errorf("[config] validate: APP_NAME is required")
	}

	return nil
}
