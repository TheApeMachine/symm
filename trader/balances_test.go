package trader

import (
	"testing"

	"github.com/theapemachine/datura"
	balancefixtures "github.com/theapemachine/symm/tests/fixtures/balances"
)

func TestBalanceSnapshotArtifact(t *testing.T) {
	source := balanceSnapshotFixture(t)
	snapshot, err := NewBalanceSnapshot(source)

	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}

	sourcePayload := source.DecryptPayload()
	sourcePayload[0] = '{'

	artifact, err := snapshot.Artifact()

	if err != nil {
		t.Fatalf("artifact failed: %v", err)
	}

	if got := datura.Peek[string](artifact, "role"); got != "balances" {
		t.Fatalf("role = %q, want balances", got)
	}

	if len(datura.Peek[[]any](artifact, "data")) == 0 {
		t.Fatal("snapshot artifact missing balance rows")
	}
}

func BenchmarkBalanceSnapshotArtifact(b *testing.B) {
	source := balanceSnapshotFixture(b)
	snapshot, err := NewBalanceSnapshot(source)

	if err != nil {
		b.Fatalf("snapshot failed: %v", err)
	}

	b.ReportAllocs()

	for b.Loop() {
		artifact, err := snapshot.Artifact()

		if err != nil {
			b.Fatalf("artifact failed: %v", err)
		}

		artifact.Release()
	}
}

func balanceSnapshotFixture(t testing.TB) *datura.Artifact {
	t.Helper()

	for artifact := range balancefixtures.NewFixture(balancefixtures.UPDATE, 1).Artifacts() {
		return artifact
	}

	t.Fatal("balance fixture did not yield an artifact")
	return nil
}
