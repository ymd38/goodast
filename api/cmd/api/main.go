// Command api は goodast の API サーバプロセス（ADR-0001）。
// Nuclei SDK には依存しない。スキャン実行は worker に分離されている。
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

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	swaggerfiles "github.com/swaggo/files"
	ginswagger "github.com/swaggo/gin-swagger"
	"go.uber.org/dig"

	"github.com/ymd38/goodast/api/internal/config"
	_ "github.com/ymd38/goodast/api/internal/docs" // swaggo 生成の OpenAPI 定義（/swagger 配信用）
	"github.com/ymd38/goodast/api/internal/credential"
	"github.com/ymd38/goodast/api/internal/db"
	"github.com/ymd38/goodast/api/internal/handler"
	"github.com/ymd38/goodast/api/internal/report"
	"github.com/ymd38/goodast/api/internal/scan"
	"github.com/ymd38/goodast/api/internal/site"
	"github.com/ymd38/goodast/secrets"
)

// @title           goodast API
// @version         0.1.0
// @description     UI起点・初心者向け OSS DAST（Nuclei ラッパー）の API。スキャン受付・サイト管理・ドメイン所有確認・認証情報の暗号化保管・ダッシュボード/レポート用データ提供。
// @description     認証情報の生値はレスポンスに一切含めない（マスク）。スキャン実行は worker プロセスに分離（ADR-0001）。
// @BasePath        /
// @schemes         http
func main() {
	if err := run(); err != nil {
		slog.Error("api terminated with error", "err", err)
		os.Exit(1)
	}
}

// run は設定読み込み・DI 構築・サーバ起動を行う。dig コンテナの構築は cmd 配下のここだけ。
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
		newRiverClient,
		newCipher,
		scan.NewService,
		site.NewRepository,
		site.DefaultVerifier,
		site.NewService,
		credential.NewRepository,
		credential.NewService,
		report.NewRepository,
		report.NewService,
		handler.NewSiteHandler,
		handler.NewScanHandler,
		handler.NewCredentialHandler,
		handler.NewDashboardHandler,
		handler.NewScanResultHandler,
		newRouter,
		newServer,
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

// newCipher は認証情報暗号化用の Cipher を生成する。鍵が不正なら起動を失敗させる（ADR-0003）。
func newCipher(cfg *config.Config) (*secrets.Cipher, error) {
	c, err := secrets.NewCipher(cfg.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("init cipher: %w", err)
	}
	return c, nil
}

// newRiverClient は insert-only の river クライアントを生成する。
// api はジョブを積むだけで処理はしない（worker に分離・ADR-0001）。Queues/Workers は持たない。
func newRiverClient(pool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	return river.NewClient(riverpgxv5.New(pool), &river.Config{})
}

type routerDeps struct {
	dig.In
	Pool       *pgxpool.Pool
	Site       *handler.SiteHandler
	Scan       *handler.ScanHandler
	Credential *handler.CredentialHandler
	Dashboard  *handler.DashboardHandler
	ScanResult *handler.ScanResultHandler
	Logger     *slog.Logger
}

// maxRequestBodyBytes は全ルート共通のリクエストボディ上限（1MiB）。
// 現状の API は小さな JSON しか受けないため保守的に絞る。
const maxRequestBodyBytes = 1 << 20

func newRouter(d routerDeps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(handler.BodyLimit(maxRequestBodyBytes))

	// feature ハンドラのルート登録。
	d.Site.RegisterRoutes(r)
	d.Scan.RegisterRoutes(r)
	d.Credential.RegisterRoutes(r)
	d.Dashboard.RegisterRoutes(r)
	d.ScanResult.RegisterRoutes(r)

	// OpenAPI(Swagger) UI: 生成済み定義（internal/docs）を /swagger で配信する。
	r.GET("/swagger/*any", ginswagger.WrapHandler(swaggerfiles.Handler))

	// liveness: プロセス死活のみ。DB は見ない。
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// readiness: DB 疎通を確認し、不可なら 503 でトラフィックから外す。
	r.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := d.Pool.Ping(ctx); err != nil {
			d.Logger.Warn("readiness check failed", "err", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	return r
}

func newServer(cfg *config.Config, r *gin.Engine) *http.Server {
	// 保守的なタイムアウトで slowloris 等によるコネクション保持・リソース枯渇を防ぐ。
	// 将来 SSE でスキャン進捗を配信する場合は、そのハンドラ内で http.ResponseController
	// により書き込み期限を個別に延長する。
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

type serveDeps struct {
	dig.In
	Server *http.Server
	Pool   *pgxpool.Pool
	Config *config.Config
	Logger *slog.Logger
}

// serve は HTTP サーバを起動し、SIGINT/SIGTERM で graceful shutdown する。
func serve(d serveDeps) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	defer d.Pool.Close()

	errCh := make(chan error, 1)
	go func() {
		d.Logger.Info("api server starting", "addr", d.Server.Addr)
		if err := d.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		d.Logger.Info("shutdown signal received, draining connections")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), d.Config.ShutdownTimeout)
	defer cancel()
	if err := d.Server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	d.Logger.Info("api server stopped")
	return nil
}
