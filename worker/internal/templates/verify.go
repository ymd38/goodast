// Package templates は nuclei-templates の導入状態を検証する。SDK 非依存の純粋ロジックに保つ。
//
// テンプレートは make nuclei-templates が固定 git tag で取得し、取得側が MarkerFile に版文字列を
// 書き込む。Nuclei 自身のインストール追跡 JSON は git clone 直取得では更新されないため、この
// 自前マーカーを版の正とする（設計: docs/superpowers/specs/2026-07-06-nuclei-templates-pinning-design.md）。
package templates

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MarkerFile は make nuclei-templates が書き込む版マーカーのファイル名。
const MarkerFile = ".goodast-templates-version"

// ErrTemplatesMissing はテンプレートディレクトリが無い／版が一致しないことを表す。
var ErrTemplatesMissing = errors.New("templates: nuclei-templates missing or version mismatch")

// Verify は dir に MarkerFile があり、その内容（trim 後）が wantVersion（trim 後）と一致することを
// 確認する。不在・不一致は ErrTemplatesMissing をラップして返す。
func Verify(dir, wantVersion string) error {
	raw, err := os.ReadFile(filepath.Join(dir, MarkerFile))
	if err != nil {
		return fmt.Errorf("%w: read marker in %q (run `make nuclei-templates`): %w", ErrTemplatesMissing, dir, err)
	}
	got := strings.TrimSpace(string(raw))
	if got != strings.TrimSpace(wantVersion) {
		return fmt.Errorf("%w: installed %q != pinned %q (run `make nuclei-templates`)", ErrTemplatesMissing, got, wantVersion)
	}
	return nil
}
