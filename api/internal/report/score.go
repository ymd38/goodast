// Package report はスキャン結果の集約（Goodast Security Score・ダッシュボード集計）を担う。
// 企画書 §5.1 のスコア設計を実装する純粋ロジックを置き、gin / net/http には依存しない。
package report

import "fmt"

// 重大度別の減点重み（企画書 §5.1）。Info は減点しない（参考情報レベル）。
// 重み値は実データで調整余地あり（PoC は §5.1 の値をそのまま採用）。
const (
	weightCritical = 40
	weightHigh     = 10
	weightMedium   = 3
	weightLow      = 1
)

// スコアバンドの下限しきい値（企画書 §5.1・80/60/40 境界）。
const (
	thresholdGood    = 80 // 80〜100
	thresholdCaution = 60 // 60〜79
	thresholdDanger  = 40 // 40〜59（未満は crisis）
)

// scoreMax / scoreMin はスコアの取り得る範囲（0〜100）。
const (
	scoreMax = 100
	scoreMin = 0
)

// SeverityCounts は 1 スキャンの重大度別 finding 件数（スコア計算用・api 固有）。
// フィールドは共有 wire 型 jobs.SeverityCounts と一致させ、repository が型変換で橋渡しする。
type SeverityCounts struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Info     int `json:"info"`
	Total    int `json:"total"`
}

// deduction は重大度別件数の加重和（減点合計）を返す。Info は減点しない。
func (c SeverityCounts) deduction() int {
	return c.Critical*weightCritical +
		c.High*weightHigh +
		c.Medium*weightMedium +
		c.Low*weightLow
}

// Band はスコアのセマンティックな区分（色は frontend の責務）。
// backend は hex を持たず、frontend が tokens.css の CSS 変数へマップする。
type Band string

const (
	BandGood    Band = "good"    // 良好（80〜100）→ --color-success
	BandCaution Band = "caution" // 要注意（60〜79）→ --color-warning
	BandDanger  Band = "danger"  // 危険（40〜59）→ --color-m-red
	BandCrisis  Band = "crisis"  // 危機（0〜39）→ --color-m-red + opacity 強調
)

// Score は Goodast Security Score（0〜100）の値オブジェクト。
// 不変条件（範囲 0〜100）をコンストラクタで強制し、不正な値のインスタンスを作れない。
type Score struct {
	v int
}

// Compute は重大度別件数から §5.1 の式でスコアを算出する。
//
//	スコア = max(0, 100 − (Critical×40 + High×10 + Medium×3 + Low×1))
//
// 正常な入力（非負カウント）では 100 − 非負 なので上限側は自明だが、DB/JSON 不整合で
// 負数カウントが混入した場合や桁あふれで 100 を超え得る。値オブジェクトの不変条件 [0,100] を
// 入力前提に依存せず守るため、下限・上限の両方をクランプする（防御的）。
func Compute(counts SeverityCounts) Score {
	v := scoreMax - counts.deduction()
	return Score{v: min(scoreMax, max(scoreMin, v))}
}

// NewScore は外部の int（DB 保存値・API 入力等）から Score を復元する。
// 範囲 0〜100 を強制し、範囲外はエラーにして不正インスタンスの生成を防ぐ。
func NewScore(v int) (Score, error) {
	if v < scoreMin || v > scoreMax {
		return Score{}, fmt.Errorf("score out of range [0,100]: %d", v)
	}
	return Score{v: v}, nil
}

// Value はスコアの整数値（0〜100）を返す。
func (s Score) Value() int { return s.v }

// Band はスコアのセマンティック区分を返す（色分けは frontend でトークンにマップ）。
func (s Score) Band() Band {
	switch {
	case s.v >= thresholdGood:
		return BandGood
	case s.v >= thresholdCaution:
		return BandCaution
	case s.v >= thresholdDanger:
		return BandDanger
	default:
		return BandCrisis
	}
}

// Label は初心者向けの日本語ラベルを返す（企画書 §5.1）。
func (s Score) Label() string {
	switch s.Band() {
	case BandGood:
		return "良好"
	case BandCaution:
		return "要注意"
	case BandDanger:
		return "危険"
	default:
		return "危機"
	}
}

// Delta は前回スコアとの差分（今回 − 前回）を返す。正なら改善・負なら悪化。
// ダッシュボード上段の「前回差分（+5↑ / -12↓）」表示に用いる。
func (s Score) Delta(prev Score) int { return s.v - prev.v }
