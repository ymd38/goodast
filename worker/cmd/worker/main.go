// Command worker は goodast のスキャンワーカープロセス（ADR-0001）。
// Nuclei SDK は internal/engine/ にのみ置く。river によるジョブ消費は ADR-0005 で追加する。
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/dig"

	"github.com/ymd38/goodast/worker/internal/config"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker terminated with error", "err", err)
		os.Exit(1)
	}
}

// run は設定読み込み・DI 構築・ヘルスサーバ起動を行う。dig コンテナの構築は cmd 配下のここだけ。
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	c := dig.New()
	providers := []any{
		func() *config.Config { return cfg },
		func() *slog.Logger { return logger },
		newPool,
		newHealthServer,
	}
	for _, p := range providers {
		if err := c.Provide(p); err != nil {
			return fmt.Errorf("provide dependency: %w", err)
		}
	}

	return c.Invoke(serve)
}

func newLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

func newPool(cfg *config.Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	poolCfg.MaxConns = cfg.DBMaxConns
	poolCfg.MinConns = cfg.DBMinConns
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.HealthCheckPeriod = time.Minute

	// pgxpool は遅延接続のため、DB 停止中でも起動でき /readyz が未準備を報告する。
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}
	return pool, nil
}

type healthDeps struct {
	dig.In
	Pool   *pgxpool.Pool
	Config *config.Config
	Logger *slog.Logger
}

func newHealthServer(d healthDeps) *http.Server {
	mux := http.NewServeMux()

	// liveness: プロセス死活のみ。
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"status":"ok"}`)
	})

	// readiness: DB 疎通を確認し、不可なら 503。
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := d.Pool.Ping(ctx); err != nil {
			d.Logger.Warn("readiness check failed", "err", err)
			writeJSON(w, http.StatusServiceUnavailable, `{"status":"unavailable"}`)
			return
		}
		writeJSON(w, http.StatusOK, `{"status":"ready"}`)
	})

	// プローブ専用だが、保守的なタイムアウトで slow-client によるリソース枯渇を防ぐ。
	return &http.Server{
		Addr:              d.Config.HealthAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

type serveDeps struct {
	dig.In
	Health *http.Server
	Pool   *pgxpool.Pool
	Config *config.Config
	Logger *slog.Logger
}

// serve はヘルスサーバを起動し、SIGINT/SIGTERM で graceful shutdown する。
func serve(d serveDeps) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	defer d.Pool.Close()

	errCh := make(chan error, 1)
	go func() {
		d.Logger.Info("worker health server starting", "addr", d.Health.Addr)
		if err := d.Health.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// NOTE: river によるジョブ消費ループは ADR-0005 でここに追加する。

	select {
	case err := <-errCh:
		return fmt.Errorf("health server error: %w", err)
	case <-ctx.Done():
		d.Logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), d.Config.ShutdownTimeout)
	defer cancel()
	if err := d.Health.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	d.Logger.Info("worker stopped")
	return nil
}
