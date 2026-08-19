package algo

import (
	"testing"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/statistic"
)

const hawkesTestEpoch = 1_786_099_200.0

func TestHawkesPublishesCountsAndMetrics(t *testing.T) {
	stream := nomagique.NewStream(Hawkes(), NewHawkesState())
	output, err := stream.Step(hawkesArrival(1, hawkesTestEpoch, 0))

	if err != nil {
		t.Fatal(err)
	}

	assertNumber(t, output, SymbolReady, 1)
	assertNumber(t, output, SymbolEventCount, 1)
	assertNumber(t, output, SymbolAlphaEventCount, 1)
	assertNumber(t, output, SymbolBetaEventCount, 0)

	if output.MustGet(SymbolLambdaAlpha) <= 0 ||
		output.MustGet(SymbolLambdaBeta) <= 0 ||
		output.MustGet(statistic.SymbolSpectralRadius) <= 0 {
		t.Fatal("Hawkes metrics should be positive")
	}
}

func TestHawkesProcessesTypedBurst(t *testing.T) {
	stream := nomagique.NewStream(Hawkes(), NewHawkesState())
	var output nomagique.Frame

	for index := 0; index < 32; index++ {
		mark := -1.0

		if index%2 != 0 {
			mark = 1
		}

		sec := hawkesTestEpoch
		nsec := float64(index * 100_000_000)

		for nsec >= 1e9 {
			sec++
			nsec -= 1e9
		}

		var err error
		output, err = stream.Step(hawkesArrival(mark, sec, nsec))

		if err != nil {
			t.Fatal(err)
		}
	}

	assertNumber(t, output, SymbolEventCount, 32)
	assertNumber(t, output, SymbolAlphaEventCount, 16)
	assertNumber(t, output, SymbolBetaEventCount, 16)
}

func TestHawkesRetainsExactTimestampCoordinates(t *testing.T) {
	stream := nomagique.NewStream(Hawkes(), NewHawkesState())
	origin := hawkesTestEpoch
	observations := []struct {
		mark float64
		sec  float64
		nsec float64
	}{
		{mark: 1, sec: origin},
		{mark: 1, sec: origin + 1},
		{mark: -1, sec: origin + 2},
		{mark: -1, sec: origin + 3},
		{mark: 1, sec: origin + 4},
	}
	var output nomagique.Frame

	for _, observation := range observations {
		var err error
		output, err = stream.Step(hawkesArrival(
			observation.mark,
			observation.sec,
			observation.nsec,
		))

		if err != nil {
			t.Fatal(err)
		}
	}

	assertNumber(t, output, SymbolObservedFromSec, origin)
	assertNumber(t, output, SymbolObservedAtSec, origin+4)
	assertNumber(t, output, SymbolAlphaEventCount, 3)
	assertNumber(t, output, SymbolBetaEventCount, 2)
}

func TestHawkesRejectsTimeRegressionTransactionally(t *testing.T) {
	stream := nomagique.NewStream(Hawkes(), NewHawkesState())

	if _, err := stream.Step(hawkesArrival(1, hawkesTestEpoch, 0)); err != nil {
		t.Fatal(err)
	}

	committed := stream.Project()

	if _, err := stream.Step(hawkesArrival(-1, hawkesTestEpoch-1, 0)); err == nil {
		t.Fatal("regressed timestamp should fail")
	}

	if !stream.Project().Equal(committed) {
		t.Fatal("failed transition changed committed Hawkes state")
	}
}

func BenchmarkHawkes(b *testing.B) {
	stream := nomagique.NewStream(Hawkes(), NewHawkesState())
	input := hawkesArrival(1, hawkesTestEpoch, 0)

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		input.Put(SymbolUnixSec, hawkesTestEpoch+float64(iteration))
		_, _ = stream.Step(input)
	}
}

func hawkesArrival(mark float64, sec float64, nsec float64) nomagique.Frame {
	input := nomagique.Frame{}
	input.Put(SymbolMark, mark)
	input.Put(SymbolUnixSec, sec)
	input.Put(SymbolUnixNsec, nsec)

	return input
}

func assertNumber(
	t *testing.T,
	frame nomagique.Frame,
	symbol nomagique.Symbol,
	want float64,
) {
	t.Helper()
	got := frame.MustGet(symbol)

	if got != want {
		t.Fatalf("symbol %d=%v; want %v", symbol, got, want)
	}
}
