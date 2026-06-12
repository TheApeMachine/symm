package cmd

import (
	"path/filepath"
	"testing"
)

func TestLiveAuditErrChecksWritableAuditPath(test *testing.T) {
	path := filepath.Join(test.TempDir(), "run", "audit.jsonl")

	if err := liveAuditErr(path); err != nil {
		test.Fatalf("expected writable audit path, got %v", err)
	}
}
