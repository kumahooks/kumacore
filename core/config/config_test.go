package config_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"kumacore/core/config"
)

// validConfig returns a Config with every required field populated so Validate passes. Individual tests mutate one
// field to exercise a specific validation branch.
func validConfig() config.Config {
	configuration := config.Config{}

	configuration.Core.Server.Host = "127.0.0.1"
	configuration.Core.Server.Port = 3000
	configuration.Core.Server.MaxBodyBytes = 64 << 20
	configuration.Core.DB.Driver = "sqlite"
	configuration.Core.DB.Path = "./data/db/kumacore.db"
	configuration.Core.Logging.Dir = "./data/logs"
	configuration.Core.Backup.Dir = "./backup"
	configuration.Core.Session.TTL = 360 * time.Hour
	configuration.Core.Session.CacheTTL = 5 * time.Minute
	configuration.Core.Worker.Enabled = false
	configuration.Core.Worker.DBPath = "./data/db/kumacore_worker.db"
	configuration.Core.Worker.PollInterval = 5 * time.Second
	configuration.Core.Worker.MaxAttempts = 3

	configuration.App.Name = "kumacore"
	configuration.App.RootDir = "."
	configuration.App.Static.Dir = "./app/web/static"

	return configuration
}

func TestValidate_ValidConfig_ReturnsNil(t *testing.T) {
	configuration := validConfig()

	if err := configuration.Validate(); err != nil {
		t.Fatalf("valid config should pass Validate, got: %v", err)
	}
}

func TestValidate_NonPositiveMaxBodyBytesFails(t *testing.T) {
	configuration := validConfig()
	configuration.Core.Server.MaxBodyBytes = 0

	err := configuration.Validate()
	if err == nil {
		t.Fatal("expected error for non-positive CORE_SERVER_MAX_BODY_BYTES, got nil")
	}

	if !strings.Contains(err.Error(), "CORE_SERVER_MAX_BODY_BYTES") {
		t.Errorf("error should mention CORE_SERVER_MAX_BODY_BYTES, got %q", err.Error())
	}
}

func TestValidate_EmptyHostFails(t *testing.T) {
	configuration := validConfig()
	configuration.Core.Server.Host = " "

	err := configuration.Validate()
	if err == nil {
		t.Fatal("expected error for empty CORE_SERVER_HOST, got nil")
	}

	if !strings.Contains(err.Error(), "CORE_SERVER_HOST") {
		t.Errorf("error should mention CORE_SERVER_HOST, got %q", err.Error())
	}
}

func TestValidate_InvalidPortFails(t *testing.T) {
	for _, port := range []int{0, 65536} {
		configuration := validConfig()
		configuration.Core.Server.Port = port

		err := configuration.Validate()
		if err == nil {
			t.Fatalf("expected error for CORE_SERVER_PORT=%d, got nil", port)
		}

		if !strings.Contains(err.Error(), "CORE_SERVER_PORT") {
			t.Errorf("error should mention CORE_SERVER_PORT, got %q", err.Error())
		}
	}
}

func TestValidate_UnsupportedDriverFails(t *testing.T) {
	configuration := validConfig()
	configuration.Core.DB.Driver = "postgres"

	err := configuration.Validate()
	if err == nil {
		t.Fatal("expected error for unsupported CORE_DB_DRIVER, got nil")
	}

	if !strings.Contains(err.Error(), "CORE_DB_DRIVER") {
		t.Errorf("error should mention CORE_DB_DRIVER, got %q", err.Error())
	}
}

func TestValidate_EmptyDBPathFails(t *testing.T) {
	configuration := validConfig()
	configuration.Core.DB.Path = ""

	err := configuration.Validate()
	if err == nil {
		t.Fatal("expected error for empty CORE_DB_PATH, got nil")
	}

	if !strings.Contains(err.Error(), "CORE_DB_PATH") {
		t.Errorf("error should mention CORE_DB_PATH, got %q", err.Error())
	}
}

func TestValidate_EmptyLogDirFails(t *testing.T) {
	configuration := validConfig()
	configuration.Core.Logging.Dir = ""

	err := configuration.Validate()
	if err == nil {
		t.Fatal("expected error for empty CORE_LOG_DIR, got nil")
	}

	if !strings.Contains(err.Error(), "CORE_LOG_DIR") {
		t.Errorf("error should mention CORE_LOG_DIR, got %q", err.Error())
	}
}

func TestValidate_WhitespaceOnlyLogDirFails(t *testing.T) {
	configuration := validConfig()
	configuration.Core.Logging.Dir = "   "

	err := configuration.Validate()
	if err == nil {
		t.Fatal("expected error for whitespace-only CORE_LOG_DIR, got nil")
	}

	if !strings.Contains(err.Error(), "CORE_LOG_DIR") {
		t.Errorf("error should mention CORE_LOG_DIR, got %q", err.Error())
	}
}

func TestValidate_NonPositiveSessionTTLFails(t *testing.T) {
	configuration := validConfig()
	configuration.Core.Session.TTL = 0

	err := configuration.Validate()
	if err == nil {
		t.Fatal("expected error for non-positive CORE_SESSION_TTL, got nil")
	}

	if !strings.Contains(err.Error(), "CORE_SESSION_TTL") {
		t.Errorf("error should mention CORE_SESSION_TTL, got %q", err.Error())
	}
}

func TestValidate_NegativeSessionCacheTTLFails(t *testing.T) {
	configuration := validConfig()
	configuration.Core.Session.CacheTTL = -time.Second

	err := configuration.Validate()
	if err == nil {
		t.Fatal("expected error for negative CORE_SESSION_CACHE_TTL, got nil")
	}

	if !strings.Contains(err.Error(), "CORE_SESSION_CACHE_TTL") {
		t.Errorf("error should mention CORE_SESSION_CACHE_TTL, got %q", err.Error())
	}
}

func TestValidate_EmptyBackupDirFails(t *testing.T) {
	configuration := validConfig()
	configuration.Core.Backup.Dir = ""

	err := configuration.Validate()
	if err == nil {
		t.Fatal("expected error for empty CORE_BACKUP_DIR, got nil")
	}

	if !strings.Contains(err.Error(), "CORE_BACKUP_DIR") {
		t.Errorf("error should mention CORE_BACKUP_DIR, got %q", err.Error())
	}
}

func TestValidate_WorkerConfigFailsWhenEnabled(t *testing.T) {
	testCases := []struct {
		name          string
		mutate        func(*config.Config)
		expectedError string
	}{
		{
			name: "empty worker database path",
			mutate: func(configuration *config.Config) {
				configuration.Core.Worker.DBPath = ""
			},
			expectedError: "CORE_WORKER_DB_PATH",
		},
		{
			name: "non-positive poll interval",
			mutate: func(configuration *config.Config) {
				configuration.Core.Worker.PollInterval = 0
			},
			expectedError: "CORE_WORKER_POLL_INTERVAL",
		},
		{
			name: "invalid max attempts",
			mutate: func(configuration *config.Config) {
				configuration.Core.Worker.MaxAttempts = 0
			},
			expectedError: "CORE_WORKER_MAX_ATTEMPTS",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			configuration := validConfig()
			configuration.Core.Worker.Enabled = true
			testCase.mutate(&configuration)

			err := configuration.Validate()
			if err == nil {
				t.Fatalf("expected error mentioning %s, got nil", testCase.expectedError)
			}

			if !strings.Contains(err.Error(), testCase.expectedError) {
				t.Errorf("error should mention %s, got %q", testCase.expectedError, err.Error())
			}
		})
	}
}

func TestValidate_EmptyAppNameFails(t *testing.T) {
	configuration := validConfig()
	configuration.App.Name = ""

	err := configuration.Validate()
	if err == nil {
		t.Fatal("expected error for empty APP_NAME, got nil")
	}

	if !strings.Contains(err.Error(), "APP_NAME") {
		t.Errorf("error should mention APP_NAME, got %q", err.Error())
	}
}

func TestValidate_EmptyRootDirFails(t *testing.T) {
	configuration := validConfig()
	configuration.App.RootDir = " "

	err := configuration.Validate()
	if err == nil {
		t.Fatal("expected error for empty APP_ROOT_DIR, got nil")
	}

	if !strings.Contains(err.Error(), "APP_ROOT_DIR") {
		t.Errorf("error should mention APP_ROOT_DIR, got %q", err.Error())
	}
}

func TestValidate_EmptyStaticDirFails(t *testing.T) {
	configuration := validConfig()
	configuration.App.Static.Dir = ""

	err := configuration.Validate()
	if err == nil {
		t.Fatal("expected error for empty APP_STATIC_DIR, got nil")
	}

	if !strings.Contains(err.Error(), "APP_STATIC_DIR") {
		t.Errorf("error should mention APP_STATIC_DIR, got %q", err.Error())
	}
}

func TestLoad_MaxBodyBytesDefault(t *testing.T) {
	previousValue, hadValue := os.LookupEnv("CORE_SERVER_MAX_BODY_BYTES")
	t.Cleanup(func() {
		if hadValue {
			_ = os.Setenv("CORE_SERVER_MAX_BODY_BYTES", previousValue)
			return
		}

		_ = os.Unsetenv("CORE_SERVER_MAX_BODY_BYTES")
	})

	if err := os.Unsetenv("CORE_SERVER_MAX_BODY_BYTES"); err != nil {
		t.Fatalf("unset CORE_SERVER_MAX_BODY_BYTES: %v", err)
	}

	configuration, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if configuration.Core.Server.MaxBodyBytes != 64<<20 {
		t.Fatalf("MaxBodyBytes: got %d, want %d", configuration.Core.Server.MaxBodyBytes, 64<<20)
	}
}
