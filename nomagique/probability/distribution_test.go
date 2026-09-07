package probability_test

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestDistributionSnapshot(t *testing.T) {
	tests.CheckDistribution(t, probability.NewDistribution())
}

func BenchmarkNewDistribution(b *testing.B) {
	node := probability.NewDistribution()
	b.ReportAllocs()

	for b.Loop() {
		input := transport.NewIO(core.From(0.0), core.From(5.0), core.From(1.0))

		if node.Next(input) == nil || node.Next(input) != nil {
			b.Fatal("expected one distribution")
		}

		if err := node.Error(); err != nil {
			b.Fatal(err)
		}
	}
}
