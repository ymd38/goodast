package config

import (
	"log/slog"
	"maps"
	"testing"
	"time"
)

// applyEnv は全関連キーを明示設定し、周囲の環境変数からテストを隔離する。
func applyEnv(t *testing.T, env map[string]string) {
	t.Helper()
	keys := []string{
		"DATABASE_URL", "GOODAST_ENCRYPTION_KEY", "WORKER_HEALTH_ADDR", "LOG_LEVEL",
		"SHUTDOWN_TIMEOUT", "DB_MAX_CONNS", "DB_MIN_CONNS",
	}
	for _, k := range keys {
		t.Setenv(k, env[k])
	}
}

func TestLoad(t *testing.T) {
	const validURL = "postgres://u:p@localhost:5432/db"
	const validKey = "test-encryption-key" // Load は非空のみ検証（形式検証は Cipher 構築側）。

	// base は必須変数（DATABASE_URL / GOODAST_ENCRYPTION_KEY）を満たす env を返す。
	base := func(extra map[string]string) map[string]string {
		env := map[string]string{"DATABASE_URL": validURL, "GOODAST_ENCRYPTION_KEY": validKey}
		maps.Copy(env, extra)
		return env
	}

	tests := []struct {
		name    string
		env     map[string]string
		wantErr bool
		check   func(t *testing.T, c *Config)
	}{
		{
			name: "minimal applies defaults",
			env:  base(nil),
			check: func(t *testing.T, c *Config) {
				if c.HealthAddr != ":9090" {
					t.Errorf("HealthAddr = %q, want :9090", c.HealthAddr)
				}
				if c.ShutdownTimeout != 30*time.Second {
					t.Errorf("ShutdownTimeout = %s, want 30s", c.ShutdownTimeout)
				}
				if c.LogLevel != slog.LevelInfo {
					t.Errorf("LogLevel = %v, want Info", c.LogLevel)
				}
				if c.DBMaxConns != 10 || c.DBMinConns != 2 {
					t.Errorf("pool conns = %d/%d, want 10/2", c.DBMaxConns, c.DBMinConns)
				}
				if c.EncryptionKey != validKey {
					t.Errorf("EncryptionKey = %q, want %q", c.EncryptionKey, validKey)
				}
			},
		},
		{
			name: "full custom values",
			env: base(map[string]string{
				"WORKER_HEALTH_ADDR": ":9999",
				"LOG_LEVEL":          "DEBUG",
				"SHUTDOWN_TIMEOUT":   "15s",
				"DB_MAX_CONNS":       "20",
				"DB_MIN_CONNS":       "5",
			}),
			check: func(t *testing.T, c *Config) {
				if c.HealthAddr != ":9999" || c.LogLevel != slog.LevelDebug ||
					c.ShutdownTimeout != 15*time.Second || c.DBMaxConns != 20 || c.DBMinConns != 5 {
					t.Errorf("unexpected config: %+v", c)
				}
			},
		},
		{name: "missing DATABASE_URL", env: map[string]string{"GOODAST_ENCRYPTION_KEY": validKey}, wantErr: true},
		{name: "missing GOODAST_ENCRYPTION_KEY", env: map[string]string{"DATABASE_URL": validURL}, wantErr: true},
		{name: "invalid LOG_LEVEL", env: base(map[string]string{"LOG_LEVEL": "trace"}), wantErr: true},
		{name: "invalid SHUTDOWN_TIMEOUT", env: base(map[string]string{"SHUTDOWN_TIMEOUT": "soon"}), wantErr: true},
		{name: "non-positive SHUTDOWN_TIMEOUT", env: base(map[string]string{"SHUTDOWN_TIMEOUT": "0s"}), wantErr: true},
		{name: "invalid DB_MAX_CONNS", env: base(map[string]string{"DB_MAX_CONNS": "lots"}), wantErr: true},
		{name: "invalid DB_MIN_CONNS", env: base(map[string]string{"DB_MIN_CONNS": "few"}), wantErr: true},
		{name: "max conns below one", env: base(map[string]string{"DB_MAX_CONNS": "0"}), wantErr: true},
		{name: "negative min conns", env: base(map[string]string{"DB_MIN_CONNS": "-1"}), wantErr: true},
		{name: "min exceeds max", env: base(map[string]string{"DB_MAX_CONNS": "2", "DB_MIN_CONNS": "5"}), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyEnv(t, tt.env)
			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if cfg.DatabaseURL != tt.env["DATABASE_URL"] {
				t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, tt.env["DATABASE_URL"])
			}
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		in      string
		want    slog.Level
		wantErr bool
	}{
		{"debug", slog.LevelDebug, false},
		{"info", slog.LevelInfo, false},
		{"warn", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"WARN", slog.LevelWarn, false},
		{"nope", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseLogLevel(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseLogLevel(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
