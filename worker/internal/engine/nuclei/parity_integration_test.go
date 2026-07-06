//go:build integration

package nuclei_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ymd38/goodast/worker/internal/engine"
	"github.com/ymd38/goodast/worker/internal/engine/nuclei"
)

// TestNucleiCLIParity は「Nuclei CLI 直接実行（ベースライン＝正解）」と「goodast エンジン経由」の
// 検出結果を突合し、goodast がガードレール適用下で findings を取りこぼしていないことを検証する
// （企画書 §10 の検知精度 DoD「欠落ゼロ」）。
//
// 前提: Juice Shop を起動し（make juiceshop-up）NUCLEI_TEST_TARGET=http://localhost:3001 を指す。
// nuclei-templates が導入済みであること。未設定なら skip する。
//
// 公平な比較のため、ベースライン CLI にも goodast の Profile と同一のフィルタ
// （tags / exclude-tags dos,intrusive / rate-limit）を適用する。goodast が意図的に落とす
// もの（破壊的タグ・スコープ外）を「欠落」と誤検出しないよう、ベースラインは scope.Allows で
// 絞った集合を正とする。
//
// 判定の粒度（フレーク耐性）:
//   - ハード fail:
//     (1) ベースラインが検出した「スコープ内 template-id」を goodast が1つでも欠くこと（脆弱性クラスの取りこぼし）。
//     (2) 共有 template-id で severity が食い違うこと。同一テンプレ由来なので severity は決定的に一致すべき。
//     (3) ベースラインの実行失敗・タイムアウト・in-scope 0 件（比較の土台＝正解が無く、素通りを防ぐ）。
//   - レポートのみ: findings 件数・URL 多重度。ステートフルな対象への2回の独立スキャンでは
//     件数・matched-URL 単位の完全一致は非決定的なため、DoD の中核は template-id 集合の包含＋severity 一致に置く。
func TestNucleiCLIParity(t *testing.T) {
	target := os.Getenv("NUCLEI_TEST_TARGET")
	if target == "" {
		t.Skip("NUCLEI_TEST_TARGET not set; skipping nuclei CLI parity test")
	}
	tags := os.Getenv("NUCLEI_TEST_TAGS")
	if tags == "" {
		tags = "misconfig,tech"
	}
	tagList := splitTags(t, tags)

	scope, err := engine.NewScope(target)
	if err != nil {
		t.Fatalf("NewScope(%q): %v", target, err)
	}

	// parity は preset ではなく明示 Profile を使い、CLI baseline と goodast のタグ集合が
	// プリセット定義のドリフトでズレないようにする（brief 記載の方針）。
	profile := engine.ScanProfile{
		Tags:        tagList,
		ExcludeTags: []string{"dos", "intrusive"},
		RateLimit:   10,
		RatePeriod:  time.Second,
	}

	// --- goodast エンジン経由の検出（未認証）---
	goodast := runGoodastScan(t, scope, profile, nil)
	t.Logf("goodast: %d findings (tags=%s, exclude=%v, rate=%d/s)",
		len(goodast), tags, profile.ExcludeTags, profile.RateLimit)

	// --- Nuclei CLI ベースライン（同一フィルタ）---
	baseline := runNucleiCLIBaseline(t, target, tagList, profile)
	t.Logf("baseline CLI: %d findings (before scope filter)", len(baseline))

	// --- ベースラインを scope.Allows で絞り、正とする ---
	inScope := make([]cliFinding, 0, len(baseline))
	for _, f := range baseline {
		if scope.Allows(f.matchedAt()) {
			inScope = append(inScope, f)
		}
	}
	t.Logf("baseline in-scope: %d findings", len(inScope))

	// in-scope が空だと missing が常に空になり検証が素通りする（正解＝ベースラインが無い）。
	// 実行失敗・テンプレ未導入・タイムアウト時の false-pass を防ぐため明示的に fail する。
	if len(inScope) == 0 {
		t.Fatalf("baseline produced no in-scope findings (raw=%d); 欠落ゼロ を検証できない — templates/target/tags を確認", len(baseline))
	}

	// --- template-id 単位の欠落判定 + severity 一致判定（ハード）---
	goodastTemplates := templateSet(goodast)
	goodastSevByID := severityByTemplate(goodast)
	baselineTemplates := map[string]bool{}
	var missing, sevMismatch []string
	for _, f := range inScope {
		baselineTemplates[f.TemplateID] = true
		if !goodastTemplates[f.TemplateID] {
			missing = append(missing, f.TemplateID)
			continue
		}
		// 共有 template-id は同一テンプレ由来。severity は決定的に一致すべき。
		if want := engine.ParseSeverity(f.Info.Severity); goodastSevByID[f.TemplateID] != want {
			sevMismatch = append(sevMismatch, fmt.Sprintf("%s: goodast=%s baseline=%s",
				f.TemplateID, goodastSevByID[f.TemplateID], want))
		}
	}
	missing = dedupSorted(missing)
	sevMismatch = dedupSorted(sevMismatch)

	// --- レポート: severity 分布・distinct template-id・extra ---
	logSeverityDistribution(t, "goodast", goodastSeverities(goodast))
	logSeverityDistribution(t, "baseline(in-scope)", cliSeverities(inScope))
	t.Logf("distinct template-ids: goodast=%d baseline(in-scope)=%d (findings 件数差はテンプレの URL 多重度による)",
		len(goodastTemplates), len(baselineTemplates))
	t.Logf("goodast template-ids: %s", strings.Join(dedupSorted(mapKeys(goodastTemplates)), ", "))
	t.Logf("baseline(in-scope) template-ids: %s", strings.Join(dedupSorted(mapKeys(baselineTemplates)), ", "))
	if extra := extraTemplates(goodastTemplates, baselineTemplates); len(extra) > 0 {
		t.Logf("goodast-only template-ids (%d, informational): %s", len(extra), strings.Join(extra, ", "))
	}

	if len(missing) > 0 {
		t.Errorf("欠落ゼロ 違反: baseline がスコープ内で検出した %d 個の template-id を goodast が欠いています:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
	if len(sevMismatch) > 0 {
		t.Errorf("severity 不一致 (%d 件): 共有 template-id で goodast と baseline の severity が食い違っています:\n  %s",
			len(sevMismatch), strings.Join(sevMismatch, "\n  "))
	}
}

// runGoodastScan は goodast エンジンで対象をスキャンし、template-id|url をキーに findings を返す。
// headers は認証後スキャンで注入する "Name: Value" 形式のヘッダ（未認証は nil）。
func runGoodastScan(t *testing.T, scope engine.Scope, profile engine.ScanProfile, headers []string) map[string]engine.Finding {
	t.Helper()
	eng := nuclei.New()
	if eng.Version() == "" {
		t.Fatal("goodast engine Version() returned empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	var (
		mu  sync.Mutex
		out = map[string]engine.Finding{}
	)
	onFinding := func(f engine.Finding) {
		mu.Lock()
		defer mu.Unlock()
		out[f.TemplateID+"|"+f.URL] = f
	}
	// 時間切れは収集済みの findings で検証を続ける（既存 TestNucleiEngineScan と同方針）。
	req := engine.ScanRequest{Scope: scope, Headers: headers, Profile: profile}
	if err := eng.Scan(ctx, req, onFinding); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("goodast Scan: %v", err)
	}
	return out
}

// cliFinding は Nuclei CLI の JSONL 出力のうち突合に必要な項目。
type cliFinding struct {
	TemplateID string `json:"template-id"`
	MatchedAt  string `json:"matched-at"`
	URL        string `json:"url"`
	Host       string `json:"host"`
	Info       struct {
		Name     string `json:"name"`
		Severity string `json:"severity"`
	} `json:"info"`
}

// matchedAt は検出箇所 URL を matched-at > url > host の優先で返す（goodast findingURL と対称）。
func (f cliFinding) matchedAt() string {
	if f.MatchedAt != "" {
		return f.MatchedAt
	}
	if f.URL != "" {
		return f.URL
	}
	return f.Host
}

// runNucleiCLIBaseline は Nuclei CLI を goodast と同一フィルタで実行し JSONL を解析して返す。
// バージョンを SDK（go.mod の v3.9.0）と一致させるため、@version を付けず module 解決に委ねる。
func runNucleiCLIBaseline(t *testing.T, target string, tags []string, profile engine.ScanProfile) []cliFinding {
	t.Helper()

	exportPath := filepath.Join(t.TempDir(), "baseline.jsonl")
	args := []string{
		"run", "github.com/projectdiscovery/nuclei/v3/cmd/nuclei",
		"-target", target,
		"-tags", strings.Join(tags, ","),
		"-exclude-tags", strings.Join(profile.ExcludeTags, ","),
		"-rate-limit", strconv.Itoa(profile.RateLimit),
		"-jsonl-export", exportPath,
		"-disable-update-check",
		"-no-interactsh", // OAST を無効化しローカル対象で決定的に走らせる
		"-silent",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Stderr = os.Stderr // 進捗・エラーはそのまま流す（findings は export ファイル側）
	err := cmd.Run()
	// タイムアウト/キャンセルは「正解が未完成」を意味する。空 baseline での素通りを防ぐため fail する。
	if ctx.Err() != nil {
		t.Fatalf("nuclei CLI baseline timed out/cancelled (%v); NUCLEI_TEST_TAGS を絞るかタイムアウトを延ばす", ctx.Err())
	}
	// 検出0件でも exit 0。非0はテンプレ未導入・引数不正等の実行失敗を示すため fail。
	if err != nil {
		t.Fatalf("nuclei CLI baseline failed: %v (args: go %s)", err, strings.Join(args, " "))
	}
	return parseJSONL(t, exportPath)
}

// parseJSONL は JSONL エクスポートを解析する。ファイル無し（検出0件）は空スライスを返す。
func parseJSONL(t *testing.T, path string) []cliFinding {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("open baseline jsonl: %v", err)
	}
	defer func() { _ = file.Close() }()

	var out []cliFinding
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // 長い JSONL 行に備えバッファ拡張
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var f cliFinding
		if err := json.Unmarshal([]byte(line), &f); err != nil {
			t.Fatalf("parse baseline jsonl line: %v (line=%q)", err, line)
		}
		out = append(out, f)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan baseline jsonl: %v", err)
	}
	return out
}

// splitTags は CSV タグを trim し空要素を除いて返す。空白混入（"a, b"）での不正タグを防ぐ。
func splitTags(t *testing.T, csv string) []string {
	t.Helper()
	var out []string
	for _, s := range strings.Split(csv, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		t.Fatalf("NUCLEI_TEST_TAGS に有効なタグがありません: %q", csv)
	}
	return out
}

// severityByTemplate は template-id → 正規化 severity の対応を返す（同一テンプレは同一 severity）。
func severityByTemplate(findings map[string]engine.Finding) map[string]engine.Severity {
	out := map[string]engine.Severity{}
	for _, f := range findings {
		out[f.TemplateID] = f.Severity
	}
	return out
}

func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func templateSet(findings map[string]engine.Finding) map[string]bool {
	set := map[string]bool{}
	for _, f := range findings {
		set[f.TemplateID] = true
	}
	return set
}

func extraTemplates(goodast, baseline map[string]bool) []string {
	var extra []string
	for id := range goodast {
		if !baseline[id] {
			extra = append(extra, id)
		}
	}
	return dedupSorted(extra)
}

func goodastSeverities(findings map[string]engine.Finding) map[string]int {
	dist := map[string]int{}
	for _, f := range findings {
		dist[string(f.Severity)]++
	}
	return dist
}

func cliSeverities(findings []cliFinding) map[string]int {
	dist := map[string]int{}
	for _, f := range findings {
		dist[string(engine.ParseSeverity(f.Info.Severity))]++
	}
	return dist
}

func logSeverityDistribution(t *testing.T, label string, dist map[string]int) {
	t.Helper()
	order := []engine.Severity{
		engine.SeverityCritical, engine.SeverityHigh, engine.SeverityMedium,
		engine.SeverityLow, engine.SeverityInfo,
	}
	parts := make([]string, 0, len(order))
	for _, s := range order {
		parts = append(parts, string(s)+"="+strconv.Itoa(dist[string(s)]))
	}
	t.Logf("%s severity: %s", label, strings.Join(parts, " "))
}

func dedupSorted(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
