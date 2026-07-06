//go:build integration

package nuclei_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ymd38/goodast/jobs"
	"github.com/ymd38/goodast/worker/internal/engine"
	"github.com/ymd38/goodast/worker/internal/engine/nuclei"
)

// TestNucleiEngineScan は Nuclei SDK を実際に呼び出す結合テスト。
// nuclei-templates が導入済みで NUCLEI_TEST_TARGET が指す対象に到達できる前提。
// 例: Juice Shop を起動し NUCLEI_TEST_TARGET=http://localhost:3001 で実行する。
//
// フルテンプレート（1万件超）は時間がかかるため、既定ではタグで部分集合に絞って
// 完走させる。タグは NUCLEI_TEST_TAGS（CSV）で上書き可能。
func TestNucleiEngineScan(t *testing.T) {
	target := os.Getenv("NUCLEI_TEST_TARGET")
	if target == "" {
		t.Skip("NUCLEI_TEST_TARGET not set; skipping nuclei integration test")
	}

	scope, err := engine.NewScope(target)
	if err != nil {
		t.Fatalf("NewScope(%q): %v", target, err)
	}

	profile := engine.PlanFor(jobs.PresetLight).Scan
	tags := os.Getenv("NUCLEI_TEST_TAGS")
	if tags == "" {
		tags = "misconfig,tech"
	}
	profile.Tags = strings.Split(tags, ",")

	eng := nuclei.New()
	if eng.Version() == "" {
		t.Fatal("Version() returned empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var (
		mu       sync.Mutex
		findings []engine.Finding
	)
	onFinding := func(f engine.Finding) {
		mu.Lock()
		defer mu.Unlock()
		findings = append(findings, f)
	}

	// 時間切れ（DeadlineExceeded）はキャンセルまでに集めた findings で検証を続ける
	// （対象/テンプレ次第でフル完走しない場合への耐性）。それ以外のエラーは失敗扱い。
	if err := eng.Scan(ctx, engine.ScanRequest{Scope: scope, Profile: profile}, onFinding); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Scan: %v", err)
	}

	// すべての検出がスコープ内（同一ホスト・非危険パス）かつ正規 severity であることを検証する。
	for _, f := range findings {
		if !scope.Allows(f.URL) {
			t.Errorf("finding URL out of scope leaked through: %q", f.URL)
		}
		switch f.Severity {
		case engine.SeverityCritical, engine.SeverityHigh, engine.SeverityMedium, engine.SeverityLow, engine.SeverityInfo:
		default:
			t.Errorf("finding %q has non-canonical severity %q", f.TemplateID, f.Severity)
		}
	}
	t.Logf("nuclei scan against %s produced %d findings", target, len(findings))
}
