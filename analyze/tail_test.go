package analyze

import (
	"testing"
)

func TestAnalyzeFileTailUsesRecentRows(t *testing.T) {
	rows := make([]map[string]any, 0, 120)

	for index := 0; index < 100; index++ {
		rows = append(rows, map[string]any{"flat": 1.0})
	}

	for index := 0; index < 20; index++ {
		rows = append(rows, map[string]any{"flat": float64(index % 2 * 2)})
	}

	path := writeDump(t, rows)

	report, err := AnalyzeFileTail("test", path, 20)

	if err != nil {
		t.Fatalf("AnalyzeFileTail: %v", err)
	}

	if report.TotalRows != 120 {
		t.Fatalf("total_rows = %d, want 120", report.TotalRows)
	}

	if report.Rows != 20 {
		t.Fatalf("rows = %d, want 20", report.Rows)
	}

	if !report.Live {
		t.Fatal("expected live report")
	}

	field := fieldByName(t, report, "flat")

	if field.Verdict != verdictFlicker {
		t.Fatalf("tail verdict = %s, want %s", field.Verdict, verdictFlicker)
	}
}

func BenchmarkAnalyzeFileTail(b *testing.B) {
	rows := make([]map[string]any, 0, LiveMaxRows)

	for index := 0; index < LiveMaxRows; index++ {
		rows = append(rows, map[string]any{
			"smooth": 1 + float64(index%10),
			"side":   "buy",
		})
	}

	path := writeDump(&testing.T{}, rows)
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		if _, err := AnalyzeFileTail("bench", path, LiveMaxRows); err != nil {
			b.Fatalf("AnalyzeFileTail: %v", err)
		}
	}
}
