//go:build integration

package scanjob_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/ymd38/goodast/jobs"
	"github.com/ymd38/goodast/secrets"
	"github.com/ymd38/goodast/worker/internal/db"
	"github.com/ymd38/goodast/worker/internal/engine"
	"github.com/ymd38/goodast/worker/internal/scanjob"
)

// testCipher は結合テスト用の Cipher（固定 32B 鍵）。
func testCipher(t *testing.T) *secrets.Cipher {
	t.Helper()
	c, err := secrets.NewCipher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	return c
}

// newTestWorker は WorkerDeps を組んで Worker を生成する（NewWorker の positional 廃止に対応）。
// Crawler は未指定（nil）。既存テストは全て light preset（Crawl.Enabled=false）を使うため、
// crawler が呼ばれることはない。
func newTestWorker(pool *pgxpool.Pool, eng engine.Engine, cipher *secrets.Cipher, logger *slog.Logger) *scanjob.Worker {
	return scanjob.NewWorker(scanjob.WorkerDeps{Queries: db.New(pool), Engine: eng, Cipher: cipher, Logger: logger})
}

// newTestWorkerWithCrawler は crawler を明示注入した Worker を生成する
// （standard/deep preset の二段配線を検証するテスト用）。
func newTestWorkerWithCrawler(pool *pgxpool.Pool, eng engine.Engine, crawler engine.Crawler, cipher *secrets.Cipher, logger *slog.Logger) *scanjob.Worker {
	return scanjob.NewWorker(scanjob.WorkerDeps{Queries: db.New(pool), Engine: eng, Crawler: crawler, Cipher: cipher, Logger: logger})
}

// fakeCrawler は二段配線検証用の決定的クローラ（実クロール不要）。
type fakeCrawler struct {
	res engine.CrawlResult
	err error
}

func (f fakeCrawler) Crawl(_ context.Context, _ engine.Scope, _ engine.CrawlPlan, _ []string) (engine.CrawlResult, error) {
	return f.res, f.err
}
func (f fakeCrawler) Version() string { return "fake/v0" }

// panicCrawler は呼ばれてはならない箇所（plan.Crawl.Enabled=false）に注入し、
// 呼び出しがあれば即座に検出する（フォールバック挙動と非呼び出しを区別するため）。
type panicCrawler struct{}

func (panicCrawler) Crawl(context.Context, engine.Scope, engine.CrawlPlan, []string) (engine.CrawlResult, error) {
	panic("crawler must not be called when plan.Crawl.Enabled=false")
}
func (panicCrawler) Version() string { return "panic/v0" }

// capturingEngine は Scan に渡された ScanRequest.Headers/Targets を記録する engine.Engine 実装。
type capturingEngine struct {
	mu      sync.Mutex
	headers []string
	targets []string
}

func (*capturingEngine) Version() string { return "capture/v0" }

func (e *capturingEngine) Scan(_ context.Context, req engine.ScanRequest, _ engine.FindingCallback) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.headers = append([]string(nil), req.Headers...)
	e.targets = append([]string(nil), req.Targets...)
	return nil
}

func (e *capturingEngine) captured() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.headers
}

func (e *capturingEngine) capturedTargets() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.targets
}

// fakeEngine は engine.Engine のテスト用実装。SDK を呼ばずに固定 findings を返す。
// （Nuclei SDK 実体の検証は worker/internal/engine/nuclei の integration テストで行う。）
type fakeEngine struct {
	findings []engine.Finding
}

func (f fakeEngine) Version() string { return "fake/v0" }

func (f fakeEngine) Scan(_ context.Context, _ engine.ScanRequest, onFinding engine.FindingCallback) error {
	for _, fd := range f.findings {
		onFinding(fd)
	}
	return nil
}

// failingEngine は常にエラーを返す engine.Engine 実装（失敗→failed 遷移の検証用）。
type failingEngine struct{}

func (failingEngine) Version() string { return "fail/v0" }

func (failingEngine) Scan(_ context.Context, _ engine.ScanRequest, _ engine.FindingCallback) error {
	return errors.New("simulated engine failure")
}

// timeoutEngine はプリセットのタイムアウト超過（context.DeadlineExceeded）を模擬する
// engine.Engine 実装（タイムアウト→即 failed 確定の検証用）。
type timeoutEngine struct{}

func (timeoutEngine) Version() string { return "timeout/v0" }

func (timeoutEngine) Scan(_ context.Context, _ engine.ScanRequest, _ engine.FindingCallback) error {
	return fmt.Errorf("simulated scan timeout: %w", context.DeadlineExceeded)
}

func countFindings(t *testing.T, pool *pgxpool.Pool, scanID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM findings WHERE scan_id = $1::uuid`, scanID).Scan(&n); err != nil {
		t.Fatalf("count findings: %v", err)
	}
	return n
}

// seedSite は site を投入し id を返す。base_url・ownership を指定できる。
func seedSite(t *testing.T, pool *pgxpool.Pool, baseURL string, verified bool) string {
	t.Helper()
	var siteID string
	if err := pool.QueryRow(context.Background(),
		// origin は worker の検証対象外（GetScanTarget は base_url を見る）。NOT NULL/UNIQUE を
		// 満たすため行ごとに一意な synthetic 値を入れる。
		`INSERT INTO sites (name, base_url, origin, ownership_verified) VALUES ($1, $2, 'itest-'||gen_random_uuid()::text, $3) RETURNING id::text`,
		"itest-"+uuid.NewString(), baseURL, verified).Scan(&siteID); err != nil {
		t.Fatalf("insert site: %v", err)
	}
	return siteID
}

// seedScan は指定 site に scan を投入し、必要なら status を上書きして scan id を返す。
func seedScan(t *testing.T, pool *pgxpool.Pool, siteID, status string) string {
	t.Helper()
	ctx := context.Background()
	var scanID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO scans (site_id) VALUES ($1::uuid) RETURNING id::text`, siteID).Scan(&scanID); err != nil {
		t.Fatalf("insert scan: %v", err)
	}
	if status != "queued" {
		if _, err := pool.Exec(ctx,
			`UPDATE scans SET status = $2, started_at = now() WHERE id = $1::uuid`, scanID, status); err != nil {
			t.Fatalf("set status: %v", err)
		}
	}
	return scanID
}

func waitForStatus(t *testing.T, pool *pgxpool.Pool, scanID, want string) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM scans WHERE id = $1::uuid`, scanID).Scan(&status); err != nil {
			t.Fatalf("query status: %v", err)
		}
		if status == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("scan %s did not reach %q; last status=%q", scanID, want, status)
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func TestScanWorker(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	eng := fakeEngine{findings: []engine.Finding{
		{TemplateID: "t-high", Title: "High issue", Severity: engine.SeverityHigh, URL: "http://localhost/x", CWE: "CWE-79"},
		{TemplateID: "t-low", Title: "Low issue", Severity: engine.SeverityLow, URL: "http://localhost/y"},
	}}

	workers := river.NewWorkers()
	river.AddWorker(workers, newTestWorker(pool, eng, testCipher(t), quiet))
	workerClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers: workers,
		Logger:  quiet,
	})
	if err != nil {
		t.Fatalf("worker client: %v", err)
	}
	insertOnly, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: quiet})
	if err != nil {
		t.Fatalf("insert-only client: %v", err)
	}
	if err := workerClient.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = workerClient.Stop(ctx) }()

	// queued ジョブが dequeue され done になり、findings と summary が保存される。
	t.Run("queued scan completes and persists findings", func(t *testing.T) {
		siteID := seedSite(t, pool, "http://localhost:3000", false)
		scanID := seedScan(t, pool, siteID, "queued")
		if _, err := insertOnly.Insert(ctx, jobs.ScanArgs{ScanID: scanID, Preset: jobs.PresetLight}, nil); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		waitForStatus(t, pool, scanID, "done")

		if count := countFindings(t, pool, scanID); count != 2 {
			t.Errorf("findings count = %d, want 2", count)
		}
		var total int
		if err := pool.QueryRow(ctx,
			`SELECT (summary_json->'findings'->>'total')::int FROM scans WHERE id = $1::uuid`, scanID).Scan(&total); err != nil {
			t.Fatalf("read summary: %v", err)
		}
		if total != 2 {
			t.Errorf("summary total = %d, want 2", total)
		}
	})

	// 既に running（途中失敗→再配送を模擬）でも冪等に done まで進み、再実行で findings が
	// 重複しない（#5）。事前に古い findings を 1 件仕込み、掃除されることも確認する。
	t.Run("running scan resumes without duplicate findings", func(t *testing.T) {
		siteID := seedSite(t, pool, "http://localhost:3000", false)
		scanID := seedScan(t, pool, siteID, "running")
		if _, err := pool.Exec(ctx,
			`INSERT INTO findings (scan_id, template_id, title, severity, url) VALUES ($1::uuid, 'stale', 'stale', 'Low', 'http://localhost/old')`,
			scanID); err != nil {
			t.Fatalf("seed stale finding: %v", err)
		}
		if _, err := insertOnly.Insert(ctx, jobs.ScanArgs{ScanID: scanID, Preset: jobs.PresetLight}, nil); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		waitForStatus(t, pool, scanID, "done")

		// 古い 1 件は掃除され、fakeEngine の 2 件のみが残る（重複・残留なし）。
		if count := countFindings(t, pool, scanID); count != 2 {
			t.Errorf("findings count after resume = %d, want 2 (no dup/stale)", count)
		}
	})

	// 公開ホストで所有未確認なら defense-in-depth で failed（ADR-0004）。
	t.Run("unverified public site is failed", func(t *testing.T) {
		siteID := seedSite(t, pool, "https://example.com", false)
		scanID := seedScan(t, pool, siteID, "queued")
		if _, err := insertOnly.Insert(ctx, jobs.ScanArgs{ScanID: scanID, Preset: jobs.PresetLight}, nil); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		waitForStatus(t, pool, scanID, "failed")
	})
}

// TestScanWorkerEngineFailureMarksFailed は、エンジン実行が最終試行で失敗した場合に
// scan が running のまま残らず failed に確定することを検証する（#4 / #7）。
func TestScanWorkerEngineFailureMarksFailed(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	workers := river.NewWorkers()
	river.AddWorker(workers, newTestWorker(pool, failingEngine{}, testCipher(t), quiet))
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers: workers,
		Logger:  quiet,
	})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	insertOnly, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: quiet})
	if err != nil {
		t.Fatalf("insert-only client: %v", err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = client.Stop(ctx) }()

	// 所有確認をスキップできる localhost を使い、エンジン実行段階の失敗だけを切り出す。
	siteID := seedSite(t, pool, "http://localhost:3000", false)
	scanID := seedScan(t, pool, siteID, "queued")
	// MaxAttempts=1 で「最終試行」を成立させ、failed 確定パスを通す。
	if _, err := insertOnly.Insert(ctx, jobs.ScanArgs{ScanID: scanID, Preset: jobs.PresetLight}, &river.InsertOpts{MaxAttempts: 1}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	waitForStatus(t, pool, scanID, "failed")
}

// TestScanWorkerTimeoutMarksFailedImmediately は、エンジン実行がタイムアウト
// （context.DeadlineExceeded）で失敗した場合、再試行枠が残っていても初回試行で
// 即 failed に確定することを検証する（恒久エラー扱い・対象サイトを叩き続けない）。
func TestScanWorkerTimeoutMarksFailedImmediately(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	workers := river.NewWorkers()
	river.AddWorker(workers, newTestWorker(pool, timeoutEngine{}, testCipher(t), quiet))
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers: workers,
		Logger:  quiet,
	})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	insertOnly, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: quiet})
	if err != nil {
		t.Fatalf("insert-only client: %v", err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = client.Stop(ctx) }()

	// engine はスタブ（timeoutEngine）でネットワークに接続しないため base_url は到達性不問。
	// 開発サーバ（Nuxt 既定 :3000 等）と紛らわしいポートは避け、確認スキップ対象の *.local を使う。
	siteID := seedSite(t, pool, "http://goodast-test.local", false)
	scanID := seedScan(t, pool, siteID, "queued")
	// MaxAttempts=3（enqueue 側の既定と同じ）で「最終試行ではない」状況を作り、
	// タイムアウトが再試行に回らず即 failed になるパスを通す。
	if _, err := insertOnly.Insert(ctx, jobs.ScanArgs{ScanID: scanID, Preset: jobs.PresetLight}, &river.InsertOpts{MaxAttempts: 3}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	waitForStatus(t, pool, scanID, "failed")

	// ジョブは初回試行のまま完了扱いになる（再試行にスケジュールされない）。
	deadline := time.Now().Add(15 * time.Second)
	for {
		var state string
		var attempt int
		if err := pool.QueryRow(ctx,
			`SELECT state, attempt FROM river_job WHERE kind = 'scan' AND args->>'scan_id' = $1`,
			scanID).Scan(&state, &attempt); err != nil {
			t.Fatalf("query river_job: %v", err)
		}
		if state == "completed" {
			if attempt != 1 {
				t.Errorf("river_job attempt = %d, want 1 (timeout must not retry)", attempt)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("river_job did not complete; last state=%q attempt=%d", state, attempt)
		}
		time.Sleep(150 * time.Millisecond)
	}
}

// TestScanWorkerInjectsSessionHeaders は、session 認証情報が設定された scan で、復号済みの
// ヘッダが engine.ScanRequest.Headers に注入されることを検証する（ADR-0003）。
func TestScanWorkerInjectsSessionHeaders(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	cipher := testCipher(t)
	capEng := &capturingEngine{}

	workers := river.NewWorkers()
	river.AddWorker(workers, newTestWorker(pool, capEng, cipher, quiet))
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers: workers,
		Logger:  quiet,
	})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	insertOnly, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: quiet})
	if err != nil {
		t.Fatalf("insert-only client: %v", err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = client.Stop(ctx) }()

	// localhost で所有確認をスキップ。site に session 認証情報を暗号化して仕込む（AAD=site_id）。
	siteID := seedSite(t, pool, "http://localhost:3000", false)
	uid := uuid.MustParse(siteID)
	enc, err := cipher.SealHeaders(secrets.Headers{
		{Name: "Cookie", Value: "session=abc"},
		{Name: "Authorization", Value: "Bearer xyz"},
	}, uid[:])
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO scan_credentials (site_id, auth_mode, enc_headers) VALUES ($1::uuid, 'session', $2)`,
		siteID, enc.Bytes()); err != nil {
		t.Fatalf("seed credentials: %v", err)
	}

	scanID := seedScan(t, pool, siteID, "queued")
	if _, err := insertOnly.Insert(ctx, jobs.ScanArgs{ScanID: scanID, Preset: jobs.PresetLight}, nil); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	waitForStatus(t, pool, scanID, "done")

	got := capEng.captured()
	want := []string{"Cookie: session=abc", "Authorization: Bearer xyz"}
	if len(got) != len(want) {
		t.Fatalf("injected headers = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("header[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestScanWorkerCredentialDecryptFailureMarksFailed は、認証情報の復号に失敗した scan が
// （恒久エラーとして）failed に確定し、かつ実行前掃除で古い findings が残らないことを検証する。
func TestScanWorkerCredentialDecryptFailureMarksFailed(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	workers := river.NewWorkers()
	// worker の cipher（鍵=9…）と別鍵で封緘し、復号を必ず失敗させる。
	river.AddWorker(workers, newTestWorker(pool, fakeEngine{}, testCipher(t), quiet))
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers: workers,
		Logger:  quiet,
	})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	insertOnly, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: quiet})
	if err != nil {
		t.Fatalf("insert-only client: %v", err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = client.Stop(ctx) }()

	wrongCipher, err := secrets.NewCipher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	siteID := seedSite(t, pool, "http://localhost:3000", false)
	uid := uuid.MustParse(siteID)
	enc, err := wrongCipher.SealHeaders(secrets.Headers{{Name: "Cookie", Value: "x"}}, uid[:])
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO scan_credentials (site_id, auth_mode, enc_headers) VALUES ($1::uuid, 'session', $2)`,
		siteID, enc.Bytes()); err != nil {
		t.Fatalf("seed credentials: %v", err)
	}

	scanID := seedScan(t, pool, siteID, "queued")
	// 前回試行の残骸を模した古い finding。復号失敗でも実行前掃除で消えることを確認する。
	if _, err := pool.Exec(ctx,
		`INSERT INTO findings (scan_id, template_id, title, severity, url) VALUES ($1::uuid, 'stale', 'stale', 'Low', 'http://localhost/old')`,
		scanID); err != nil {
		t.Fatalf("seed stale finding: %v", err)
	}
	if _, err := insertOnly.Insert(ctx, jobs.ScanArgs{ScanID: scanID, Preset: jobs.PresetLight}, nil); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	waitForStatus(t, pool, scanID, "failed")

	if count := countFindings(t, pool, scanID); count != 0 {
		t.Errorf("findings after decrypt-failure = %d, want 0 (stale cleared)", count)
	}
}

// getSummary は scan の summary_json を読み jobs.ScanSummary にデコードする。
func getSummary(t *testing.T, pool *pgxpool.Pool, scanID string) jobs.ScanSummary {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(context.Background(),
		`SELECT summary_json FROM scans WHERE id = $1::uuid`, scanID).Scan(&raw); err != nil {
		t.Fatalf("read summary: %v", err)
	}
	var sum jobs.ScanSummary
	if err := json.Unmarshal(raw, &sum); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	return sum
}

// TestWorkerTwoPhaseDiscovery は crawl（standard preset・有効）→ 動的タイムアウト → scan の
// 二段配線を検証する。fake crawler が返す発見 URL 群が診断対象（engine.ScanRequest.Targets）に
// 供給され、summary.Discovery に集計が載ることを確認する（実クロール不要）。
func TestWorkerTwoPhaseDiscovery(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	fake := fakeCrawler{res: engine.CrawlResult{
		URLs:      []string{"http://localhost:3001/", "http://localhost:3001/products"},
		FormCount: 2,
	}}
	capEng := &capturingEngine{}

	workers := river.NewWorkers()
	river.AddWorker(workers, newTestWorkerWithCrawler(pool, capEng, fake, testCipher(t), quiet))
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers: workers,
		Logger:  quiet,
	})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	insertOnly, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: quiet})
	if err != nil {
		t.Fatalf("insert-only client: %v", err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = client.Stop(ctx) }()

	// localhost で所有確認をスキップ。standard preset は Crawl.Enabled=true。
	siteID := seedSite(t, pool, "http://localhost:3001", false)
	scanID := seedScan(t, pool, siteID, "queued")
	if _, err := insertOnly.Insert(ctx, jobs.ScanArgs{ScanID: scanID, Preset: jobs.PresetStandard}, nil); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	waitForStatus(t, pool, scanID, "done")

	sum := getSummary(t, pool, scanID)
	if sum.Discovery == nil || sum.Discovery.URLCount != 2 || sum.Discovery.FormCount != 2 {
		t.Fatalf("Discovery = %+v; want url=2 form=2", sum.Discovery)
	}

	gotTargets := capEng.capturedTargets()
	wantTargets := []string{"http://localhost:3001/", "http://localhost:3001/products"}
	if len(gotTargets) != len(wantTargets) {
		t.Fatalf("targets = %v, want %v", gotTargets, wantTargets)
	}
	for i := range wantTargets {
		if gotTargets[i] != wantTargets[i] {
			t.Errorf("target[%d] = %q, want %q", i, gotTargets[i], wantTargets[i])
		}
	}
}

// TestWorkerCrawlDisabledFallsBackToSingleURL は light preset（Crawl.Enabled=false）で
// crawler が一切呼ばれず、診断対象が scope の単一 URL にフォールバックし、
// summary.Discovery が nil のままであることを検証する。
func TestWorkerCrawlDisabledFallsBackToSingleURL(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	// crawl 無効時は呼ばれてはならないため、呼ばれたら panic する crawler を仕込む
	// （呼ばれた場合は Work がエラー/パニックで返り waitForStatus が done に到達せず検出できる）。
	capEng := &capturingEngine{}

	workers := river.NewWorkers()
	river.AddWorker(workers, newTestWorkerWithCrawler(pool, capEng, panicCrawler{}, testCipher(t), quiet))
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers: workers,
		Logger:  quiet,
	})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	insertOnly, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: quiet})
	if err != nil {
		t.Fatalf("insert-only client: %v", err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = client.Stop(ctx) }()

	siteID := seedSite(t, pool, "http://localhost:3002", false)
	scanID := seedScan(t, pool, siteID, "queued")
	if _, err := insertOnly.Insert(ctx, jobs.ScanArgs{ScanID: scanID, Preset: jobs.PresetLight}, nil); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	waitForStatus(t, pool, scanID, "done")

	sum := getSummary(t, pool, scanID)
	if sum.Discovery != nil {
		t.Fatalf("Discovery = %+v; want nil (crawl disabled)", sum.Discovery)
	}

	gotTargets := capEng.capturedTargets()
	wantTargets := []string{"http://localhost:3002"}
	if len(gotTargets) != len(wantTargets) || (len(gotTargets) > 0 && gotTargets[0] != wantTargets[0]) {
		t.Fatalf("targets = %v, want %v (single-URL fallback)", gotTargets, wantTargets)
	}
}
