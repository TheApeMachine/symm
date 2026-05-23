package replay

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFramesReadsJSONL(t *testing.T) {
	path := filepath.Join("fixtures", "sample.jsonl")

	frames, err := LoadFrames(path)

	if err != nil {
		t.Fatalf("load frames: %v", err)
	}

	if len(frames) < 3 {
		t.Fatalf("expected at least 3 frames, got %d", len(frames))
	}
}

func TestReadFramesRejectsEmptySource(t *testing.T) {
	_, err := ReadFrames(strings.NewReader("\n\n"))

	if err == nil {
		t.Fatal("expected error for empty replay source")
	}
}

func BenchmarkReadFrames(b *testing.B) {
	payload := []byte(`{"channel":"trade","type":"update","data":[]}` + "\n" +
		`{"channel":"book","type":"update","data":[]}` + "\n")

	b.ReportAllocs()

	for b.Loop() {
		if _, err := ReadFrames(bytes.NewReader(payload)); err != nil {
			b.Fatal(err)
		}
	}
}
