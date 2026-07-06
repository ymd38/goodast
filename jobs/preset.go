package jobs

import "errors"

// Preset はスキャンの範囲・深さのプリセット（企画書 §6-2 のウィザード選択）。
// api（検証・保存）と worker（Timeout・scan config）が共有するため、SDK 非依存の
// 本モジュールに一元定義する（ADR-0002: api は engine を import できない）。
type Preset string

const (
	PresetLight    Preset = "light"    // 軽量: 素早い基本チェック
	PresetStandard Preset = "standard" // 標準: 実用的な中間（デフォルト）
	PresetDeep     Preset = "deep"     // 詳細: 広いタグ集合（タグ有界で全テンプレは回さない）
)

// DefaultPreset は preset 省略時に採用する安全な既定値。
const DefaultPreset = PresetStandard

// ErrInvalidPreset は未知の preset 文字列を表す。
var ErrInvalidPreset = errors.New("jobs: invalid scan preset")

// ParsePreset は文字列を Preset に変換する。空文字は DefaultPreset を返し（省略許容）、
// 未知値は ErrInvalidPreset を返す。DB CHECK 制約・HTTP 400 と二重に不正値を弾く。
func ParsePreset(s string) (Preset, error) {
	switch Preset(s) {
	case PresetLight, PresetStandard, PresetDeep:
		return Preset(s), nil
	case "":
		return DefaultPreset, nil
	default:
		return "", ErrInvalidPreset
	}
}

// String は Preset の文字列表現を返す。
func (p Preset) String() string { return string(p) }
