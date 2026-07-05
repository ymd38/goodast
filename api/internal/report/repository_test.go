package report

import "testing"

// TestDecodeSummaryCounts は summary_json のデコード契約を固定する。
// worker（scanjob.scanSummary）は counts を "findings" キーの下にネストして書き込むため、
// フラット形を直接デコードすると無言で全 0 になる（この不一致が本番でカウント 0 化を招いた）。
func TestDecodeSummaryCounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    SeverityCounts
		wantErr bool
	}{
		{
			name: "worker が書き込むネスト形から counts を取り出す",
			raw:  `{"findings":{"critical":1,"high":2,"medium":3,"low":4,"info":13,"total":23}}`,
			want: SeverityCounts{Critical: 1, High: 2, Medium: 3, Low: 4, Info: 13, Total: 23},
		},
		{
			name: "findings キーが無い（フラット）形は全 0 になる（契約: ネスト必須）",
			raw:  `{"critical":1,"high":2,"medium":3,"low":4,"info":13,"total":23}`,
			want: SeverityCounts{},
		},
		{
			name:    "不正な JSON はエラー",
			raw:     `{"findings":`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := decodeSummaryCounts([]byte(tt.raw))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("decodeSummaryCounts(%q): エラーを期待したが nil", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeSummaryCounts(%q): 予期せぬエラー: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Errorf("decodeSummaryCounts(%q) = %+v, want %+v", tt.raw, got, tt.want)
			}
		})
	}
}
