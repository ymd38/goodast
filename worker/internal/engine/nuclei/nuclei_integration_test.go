//go:build integration

package nuclei_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ymd38/goodast/worker/internal/engine"
	"github.com/ymd38/goodast/worker/internal/engine/nuclei"
)

// TestNucleiEngineScan は Nuclei SDK を実際に呼び出す結合テスト。
// nuclei-templates が導入済みで NUCLEI_TEST_TARGET が指す対象に到達できる前提。
// 例: Juice Shop を起動し NUCLEI_TEST_TARGET=http://localhost:3000 で実行する。
func TestNucleiEngineScan(t *testing.T) {
	target := os.Getenv("NUCLEI_TEST_TARGET")
	if target == "" {
		t.Skip("NUCLEI_TEST_TARGET not set; skipping nuclei integration test")
	}

	scope, err := engine.NewScope(target)
	if err != nil {
		t.Fatalf("NewScope(%q): %v", target, err)
	}

	eng := nuclei.New(nuclei.DefaultConfig())
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

	if err := eng.Scan(ctx, engine.ScanRequest{Scope: scope}, onFinding); err != nil {
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
