package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/analyze"
	"github.com/theapemachine/symm/rawdump"
)

func TestSignalFromDumpPath(t *testing.T) {
	cases := map[string]string{
		"/tmp/runs/cvd_raw.jsonl":      "cvd",
		"/tmp/runs/pumpdump_raw.jsonl": "pumpdump",
		"/tmp/runs/not_a_dump.jsonl":   "",
	}

	for path, want := range cases {
		if got := signalFromDumpPath(path); got != want {
			t.Errorf("signalFromDumpPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestDiagnosticFrameAddsChartKey(t *testing.T) {
	report, err := analyze.AnalyzeFile("test", writeAnalyzerDump(t, []map[string]any{
		{"x": 1.0},
	}), 0)

	if err != nil {
		t.Fatalf("AnalyzeFile: %v", err)
	}

	frame, err := diagnosticFrame(report)

	if err != nil {
		t.Fatalf("diagnosticFrame: %v", err)
	}

	if frame["chart"] != "diagnostic" {
		t.Fatalf("chart = %v, want diagnostic", frame["chart"])
	}

	if frame["signal"] != "test" {
		t.Fatalf("signal = %v, want test", frame["signal"])
	}
}

func TestDiagnosticsWatcherDebouncesAnalysis(t *testing.T) {
	dir := t.TempDir()
	viper.Set("signals.raw_dump_dir", dir)
	t.Cleanup(func() {
		viper.Set("signals.raw_dump_dir", "")
	})

	path := filepath.Join(dir, "cvd_raw.jsonl")

	if err := os.WriteFile(path, []byte(`{"signed":1}`+"\n"), 0o644); err != nil {
		t.Fatalf("write dump: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := &Hub{
		ctx:     ctx,
		cancel:  cancel,
		clients: &sync.Map{},
		server:  &http.Server{},
	}

	diagnostics := startDiagnosticsWatcher(hub)

	if diagnostics == nil {
		t.Fatal("expected diagnostics watcher")
	}

	defer diagnostics.Close()

	diagnostics.schedule("cvd")
	time.Sleep(diagnosticsDebounce + 300*time.Millisecond)

	report, err := analyze.AnalyzeFileTail("cvd", path, analyze.LiveMaxRows)

	if err != nil {
		t.Fatalf("AnalyzeFileTail: %v", err)
	}

	if report.Rows != 1 {
		t.Fatalf("rows = %d, want 1", report.Rows)
	}
}

func TestListRawDumps(t *testing.T) {
	dir := t.TempDir()
	viper.Set("signals.raw_dump_dir", dir)
	t.Cleanup(func() {
		viper.Set("signals.raw_dump_dir", "")
	})

	if err := os.WriteFile(filepath.Join(dir, "fluid_raw.jsonl"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write dump: %v", err)
	}

	dumps, err := listRawDumps()

	if err != nil {
		t.Fatalf("listRawDumps: %v", err)
	}

	if len(dumps) != 1 || dumps[0].Signal != "fluid" {
		t.Fatalf("dumps = %+v, want single fluid entry", dumps)
	}

	if dumps[0].File != filepath.Join(rawdump.Dir(), "fluid_raw.jsonl") {
		t.Fatalf("file = %q", dumps[0].File)
	}
}

func writeAnalyzerDump(t *testing.T, rows []map[string]any) string {
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
