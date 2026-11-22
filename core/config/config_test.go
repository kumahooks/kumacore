package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func validConfiguration() Config {
	var configuration Config

	configuration.Core.Server.Host = "127.0.0.1"
	configuration.Core.Server.Port = 3000
	configuration.Core.Server.MaxBodyBytes = 64 << 20
	configuration.Core.DB.Driver = "sqlite"
	configuration.Core.DB.Path = ":memory:"
	configuration.Core.Logging.Dir = "logs"
	configuration.Core.Session.TTL = time.Hour
	configuration.Core.Session.CacheTTL = time.Minute
	configuration.Core.Backup.Dir = "backup"
	configuration.App.Name = "test"
	configuration.App.RootDir = "."
	configuration.App.Static.Dir = "static"

	return configuration
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

	configuration, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if configuration.Core.Server.MaxBodyBytes != 64<<20 {
		t.Fatalf("MaxBodyBytes: got %d, want %d", configuration.Core.Server.MaxBodyBytes, 64<<20)
	}
}

func TestValidate_MaxBodyBytes(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		maxBodyBytes int64
		wantError    bool
	}{
		{name: "positive", maxBodyBytes: 1, wantError: false},
		{name: "zero", maxBodyBytes: 0, wantError: true},
		{name: "negative", maxBodyBytes: -1, wantError: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			configuration := validConfiguration()
			configuration.Core.Server.MaxBodyBytes = testCase.maxBodyBytes

			err := configuration.Validate()

			if testCase.wantError {
				if err == nil || !strings.Contains(err.Error(), "CORE_SERVER_MAX_BODY_BYTES") {
					t.Fatalf("Validate: got %v, want CORE_SERVER_MAX_BODY_BYTES error", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}
