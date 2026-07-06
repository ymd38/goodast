package jobs

import (
	"encoding/json"
	"testing"
)

func TestScanArgsCarriesPreset(t *testing.T) {
	raw, err := json.Marshal(ScanArgs{ScanID: "abc", Preset: PresetDeep})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out ScanArgs
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ScanID != "abc" || out.Preset != PresetDeep {
		t.Fatalf("got %+v", out)
	}
}

func TestScanArgsKind(t *testing.T) {
	if (ScanArgs{}).Kind() != "scan" {
		t.Fatalf("kind changed")
	}
}
