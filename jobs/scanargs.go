// Package jobs は api（enqueue 側）と worker（処理側）が共有するジョブ契約を定義する。
// river は Kind 文字列 + JSON ペイロードでジョブと Worker を照合するため、契約をここに
// 一元化して両モジュール間のドリフト（= 無言のジョブ失敗）を防ぐ。
package jobs

// ScanArgs はスキャン実行ジョブの引数。worker は ScanID から scan / site / credentials を
// DB ロードするため、ペイロードは ScanID のみに保つ。
type ScanArgs struct {
	ScanID string `json:"scan_id"`
}

// Kind は river のジョブ種別識別子。変更すると既存ジョブと互換でなくなるため固定する。
func (ScanArgs) Kind() string { return "scan" }
