// Command worker は goodast のスキャンワーカープロセス（ADR-0001）。
// Nuclei SDK は internal/engine/ にのみ置く。river でジョブを消費する（ADR-0005）。
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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"go.uber.org/dig"

	"github.com/ymd38/goodast/secrets"
	"github.com/ymd38/goodast/worker/internal/config"
	"github.com/ymd38/goodast/worker/internal/db"
	"github.com/ymd38/goodast/worker/internal/engine"
	"github.com/ymd38/goodast/worker/internal/engine/nuclei"
	"github.com/ymd38/goodast/worker/internal/scanjob"
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
		func(pool *pgxpool.Pool) *db.Queries { return db.New(pool) },
		// engine.Engine の唯一の実装は Nuclei（ADR-0002）。実行パラメータは per-scan の
		// ScanRequest.Profile から渡すため、配線時に Config は不要。
		func() engine.Engine { return nuclei.New() },
		newCipher,
		scanjob.NewWorker,
		newRiverClient,
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

// newCipher は認証情報復号用の Cipher を生成する。鍵が不正なら起動を失敗させる（ADR-0003）。
func newCipher(cfg *config.Config) (*secrets.Cipher, error) {
	c, err := secrets.NewCipher(cfg.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("init cipher: %w", err)
	}
	return c, nil
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

// newRiverClient は scan ジョブを処理する river クライアントを生成する。
func newRiverClient(pool *pgxpool.Pool, sw *scanjob.Worker, logger *slog.Logger) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, sw)

	// スキャンは高負荷なため同時実行数は保守的に抑える（ADR の保守的レート方針）。
	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 5}},
		Workers: workers,
		Logger:  logger,
	})
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
	River  *river.Client[pgx.Tx]
	Pool   *pgxpool.Pool
	Config *config.Config
	Logger *slog.Logger
}

// serve は river クライアントとヘルスサーバを起動し、SIGINT/SIGTERM で graceful shutdown する。
func serve(d serveDeps) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	defer d.Pool.Close()

	if err := d.River.Start(ctx); err != nil {
		return fmt.Errorf("start river client: %w", err)
	}
	d.Logger.Info("river client started")

	errCh := make(chan error, 1)
	go func() {
		d.Logger.Info("worker health server starting", "addr", d.Health.Addr)
		if err := d.Health.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// シグナル経路・ヘルスサーバ異常経路のどちらでも、以降の共通 shutdown を必ず通す。
	var runErr error
	select {
	case err := <-errCh:
		runErr = fmt.Errorf("health server error: %w", err)
	case <-ctx.Done():
		d.Logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), d.Config.ShutdownTimeout)
	defer cancel()
	// river を graceful に停止し、実行中ジョブの完了を待つ（両経路で必ず実行する）。
	if err := d.River.Stop(shutdownCtx); err != nil {
		d.Logger.Warn("river stop error", "err", err)
	}
	if err := d.Health.Shutdown(shutdownCtx); err != nil && runErr == nil {
		runErr = fmt.Errorf("graceful shutdown failed: %w", err)
	}
	if runErr == nil {
		d.Logger.Info("worker stopped")
	}
	return runErr
}
