package response

import (
	"testing"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

func TestTreeHandlerStoresRoleScopeTimestampArtifact(t *testing.T) {
	tree := dmt.NewTree("")
	handler := NewTreeHandler(tree)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("ticker")
	artifact.WithScope("update")
	artifact.WithPayload([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":1}]}`))
	artifact.SetTimestamp(time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC).UnixNano())
	defer artifact.Release()

	handler.Send(artifact)

	if got := countTreeArtifacts(tree, []byte("ticker/update/2026/06/29/12/00/00")); got != 1 {
		t.Fatalf("canonical tree artifacts = %d, want 1", got)
	}
	if got := countTreeArtifacts(tree, []byte("ticker/2026/06/29/12/00/00")); got != 0 {
		t.Fatalf("old role/timestamp tree artifacts = %d, want 0", got)
	}
	if got := countTreeArtifacts(tree, []byte("ticker/BTC/USD")); got != 0 {
		t.Fatalf("symbol-scoped tree artifacts = %d, want 0", got)
	}
}

func countTreeArtifacts(tree *dmt.Tree, prefix []byte) int {
	count := 0

	for range tree.Seek(prefix) {
		count++
	}

	return count
}
