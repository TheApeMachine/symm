package analyze

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// writeDump renders rows as JSONL into a temp file and returns the path.
func writeDump(t *testing.T, rows []map[string]any) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "dump.jsonl")
	file, err := os.Create(path)

	if err != nil {
		t.Fatalf("create dump: %v", err)
	}

	defer file.Close()

	encoder := json.NewEncoder(file)

	for _, row := range rows {
		if err := encoder.Encode(row); err != nil {
			t.Fatalf("encode row: %v", err)
		}
	}

	return path
}

func fieldByName(t *testing.T, report *Report, name string) FieldReport {
	t.Helper()

	for _, field := range report.Fields {
		if field.Name == name {
			return field
		}
	}

	t.Fatalf("field %q not found in report", name)

	return FieldReport{}
}

func TestAnalyzeClassifiesFieldShapes(t *testing.T) {
	const count = 1000

	rows := make([]map[string]any, 0, count)

	for index := 0; index < count; index++ {
		rows = append(rows, map[string]any{
			"flat":    1.0,                                       // constant ⇒ DEAD
			"flicker": float64(index % 2 * 2),                    // 0,2,0,2 ⇒ FLICKERING
			"smooth":  1 + 0.5*(1+math.Sin(float64(index)/50.0)), // smooth 1..2 ⇒ HEALTHY
			"side":    "buy",                                     // constant ⇒ CONSTANT
			"flips":   pick(index%2 == 0, "a", "b"),              // alternating ⇒ UNSTABLE
		})
	}

	path := writeDump(t, rows)

	report, err := AnalyzeFile("test", path, 0)

	if err != nil {
		t.Fatalf("AnalyzeFile: %v", err)
	}

	if report.Rows != count {
		t.Fatalf("rows = %d, want %d", report.Rows, count)
	}

	cases := map[string]string{
		"flat":    verdictDead,
		"flicker": verdictFlicker,
		"smooth":  verdictHealthy,
		"side":    verdictConstant,
		"flips":   verdictUnstable,
	}

	for name, want := range cases {
		field := fieldByName(t, report, name)

		if field.Verdict != want {
			t.Errorf("field %q: verdict = %s, want %s (autocorr=%.3f crossing=%.3f)",
				name, field.Verdict, want, field.Lag1Autocorr, field.MeanCrossingRate)
		}
	}
}

func TestNumericBatteryMatchesKnownSeries(t *testing.T) {
	// A clean alternating 0/2 series: mean 1, every step crosses the mean, and
	// lag-1 autocorrelation is -1.
	rows := make([]map[string]any, 0, 100)

	for index := 0; index < 100; index++ {
		rows = append(rows, map[string]any{"x": float64(index % 2 * 2)})
	}

	report, err := AnalyzeFile("test", writeDump(t, rows), 0)

	if err != nil {
		t.Fatalf("AnalyzeFile: %v", err)
	}

	field := fieldByName(t, report, "x")

	if math.Abs(field.Mean-1.0) > 1e-9 {
		t.Errorf("mean = %v, want 1.0", field.Mean)
	}

	if math.Abs(field.MeanCrossingRate-1.0) > 1e-9 {
		t.Errorf("mean-crossing rate = %v, want 1.0", field.MeanCrossingRate)
	}

	// A perfectly alternating series of length n has lag-1 autocorrelation
	// -(n-1)/n, which approaches but never reaches -1. Assert it is strongly
	// negative (clearly oscillating) rather than pinning an exact value.
	if field.Lag1Autocorr > -0.95 {
		t.Errorf("lag-1 autocorr = %v, want strongly negative (< -0.95)", field.Lag1Autocorr)
	}
}

func TestFieldReportsAreAlphabetical(t *testing.T) {
	rows := []map[string]any{
		{"zebra": 1.0, "alpha": 2.0, "middle": 3.0},
		{"alpha": 4.0, "zebra": 5.0, "middle": 6.0},
	}

	path := writeDump(t, rows)

	report, err := AnalyzeFile("test", path, 0)

	if err != nil {
		t.Fatalf("AnalyzeFile: %v", err)
	}

	want := []string{"alpha", "middle", "zebra"}

	if len(report.Fields) != len(want) {
		t.Fatalf("fields = %d, want %d", len(report.Fields), len(want))
	}

	for index, name := range want {
		if report.Fields[index].Name != name {
			t.Fatalf("field[%d] = %q, want %q", index, report.Fields[index].Name, name)
		}
	}

	tailReport, err := AnalyzeFileTail("test", path, 10)

	if err != nil {
		t.Fatalf("AnalyzeFileTail: %v", err)
	}

	for index, name := range want {
		if tailReport.Fields[index].Name != name {
			t.Fatalf("tail field[%d] = %q, want %q", index, tailReport.Fields[index].Name, name)
		}
	}
}

func TestEmptyFileIsHandled(t *testing.T) {
	report, err := AnalyzeFile("test", writeDump(t, nil), 0)

	if err != nil {
		t.Fatalf("AnalyzeFile on empty: %v", err)
	}

	if report.Rows != 0 {
		t.Errorf("rows = %d, want 0", report.Rows)
	}

	if report.Headline == "" {
		t.Error("expected a headline even for an empty file")
	}
}

func pick(condition bool, whenTrue, whenFalse string) string {
	if condition {
		return whenTrue
	}

	return whenFalse
}
