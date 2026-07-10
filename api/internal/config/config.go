// Package config は API サーバの実行時設定を環境変数から読み込む（12-factor）。
// 必須変数の欠落・不正値は Load の時点でエラーにし、起動を失敗させる。
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ymd38/goodast/api/internal/target"
)

// Config は API サーバの実行時設定。環境変数から Load で構築する。
type Config struct {
	DatabaseURL     string
	EncryptionKey   string // GOODAST_ENCRYPTION_KEY（base64 32B）。認証情報の暗号化に使う（ADR-0003）。
	Addr            string
	ShutdownTimeout time.Duration
	LogLevel        slog.Level
	DBMaxConns      int32
	DBMinConns      int32
	// SelfOrigins は GOODAST 自身の origin（ドメイン+ポート）集合。ここに一致する
	// 対象は自己スキャン防止のため登録を拒否する。
	SelfOrigins target.SelfOrigins
}

const (
	defaultAddr            = ":8080"
	defaultShutdownTimeout = 30 * time.Second
	defaultDBMaxConns      = 10
	defaultDBMinConns      = 2
	// defaultSelfOrigins は GOODAST の web UI（:3000）と api（:8080）の既定 origin。
	// ループバック別名（127.0.0.1 / ::1）は正規化で localhost に畳み込まれるため同時に覆う。
	defaultSelfOrigins = "localhost:3000,localhost:8080"
)

// Load は環境変数から Config を構築し検証する。
// 検証に失敗した場合はエラーを返し、サイレントなデフォルトでの起動継続を防ぐ。
func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	// 認証情報の暗号鍵は必須（ADR-0003）。鍵形式の検証は起動時の Cipher 構築に委ねる。
	encKey := os.Getenv("GOODAST_ENCRYPTION_KEY")
	if encKey == "" {
		return nil, fmt.Errorf("GOODAST_ENCRYPTION_KEY is required")
	}

	level, err := parseLogLevel(getEnv("LOG_LEVEL", "info"))
	if err != nil {
		return nil, err
	}

	timeout, err := time.ParseDuration(getEnv("SHUTDOWN_TIMEOUT", defaultShutdownTimeout.String()))
	if err != nil {
		return nil, fmt.Errorf("invalid SHUTDOWN_TIMEOUT: %w", err)
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("SHUTDOWN_TIMEOUT must be positive: %s", timeout)
	}

	maxConns, err := parseInt32("DB_MAX_CONNS", defaultDBMaxConns)
	if err != nil {
		return nil, err
	}
	minConns, err := parseInt32("DB_MIN_CONNS", defaultDBMinConns)
	if err != nil {
		return nil, err
	}
	if maxConns < 1 {
		return nil, fmt.Errorf("DB_MAX_CONNS must be >= 1: %d", maxConns)
	}
	if minConns < 0 {
		return nil, fmt.Errorf("DB_MIN_CONNS must be >= 0: %d", minConns)
	}
	if minConns > maxConns {
		return nil, fmt.Errorf("DB_MIN_CONNS (%d) must not exceed DB_MAX_CONNS (%d)", minConns, maxConns)
	}

	selfOrigins, err := target.NewSelfOrigins(strings.Split(getEnv("GOODAST_SELF_ORIGINS", defaultSelfOrigins), ","))
	if err != nil {
		return nil, fmt.Errorf("invalid GOODAST_SELF_ORIGINS: %w", err)
	}

	return &Config{
		DatabaseURL:     dbURL,
		EncryptionKey:   encKey,
		Addr:            getEnv("API_ADDR", defaultAddr),
		ShutdownTimeout: timeout,
		LogLevel:        level,
		DBMaxConns:      maxConns,
		DBMinConns:      minConns,
		SelfOrigins:     selfOrigins,
	}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseLogLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid LOG_LEVEL: %q", s)
	}
}

func parseInt32(key string, fallback int32) (int32, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return int32(n), nil
}
