package algo

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/types"
)

const hawkesTestEpoch = 1_786_099_200.0

func TestHawkesPublishesCountsAndMetrics(t *testing.T) {
	stream := types.NewStream(Hawkes(), statistic.NewHawkesState())
	output := stream.Step(hawkesArrival(1, hawkesTestEpoch, 0))

	if output.Err != nil {
		t.Fatal(output.Err)
	}

	assertNumber(t, output, statistic.SymbolReady, 1)
	assertNumber(t, output, statistic.SymbolEventCount, 1)
	assertNumber(t, output, statistic.SymbolAlphaEventCount, 1)
	assertNumber(t, output, statistic.SymbolBetaEventCount, 0)

	if output.MustGet(statistic.SymbolLambdaAlpha) <= 0 ||
		output.MustGet(statistic.SymbolLambdaBeta) <= 0 ||
		output.MustGet(statistic.SymbolSpectralRadius) <= 0 {
		t.Fatal("Hawkes metrics should be positive")
	}
}

func TestHawkesProcessesTypedBurst(t *testing.T) {
	stream := types.NewStream(Hawkes(), statistic.NewHawkesState())
	var output types.Frame

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

		output = stream.Step(hawkesArrival(mark, sec, nsec))

		if output.Err != nil {
			t.Fatal(output.Err)
		}
	}

	assertNumber(t, output, statistic.SymbolEventCount, 32)
	assertNumber(t, output, statistic.SymbolAlphaEventCount, 16)
	assertNumber(t, output, statistic.SymbolBetaEventCount, 16)
}

func TestHawkesRetainsExactTimestampCoordinates(t *testing.T) {
	stream := types.NewStream(Hawkes(), statistic.NewHawkesState())
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
	var output types.Frame

	for _, observation := range observations {
		output = stream.Step(hawkesArrival(
			observation.mark,
			observation.sec,
			observation.nsec,
		))

		if output.Err != nil {
			t.Fatal(output.Err)
		}
	}

	assertNumber(t, output, statistic.SymbolObservedFromSec, origin)
	assertNumber(t, output, statistic.SymbolObservedAtSec, origin+4)
	assertNumber(t, output, statistic.SymbolAlphaEventCount, 3)
	assertNumber(t, output, statistic.SymbolBetaEventCount, 2)
}

func TestHawkesRejectsTimeRegressionTransactionally(t *testing.T) {
	stream := types.NewStream(Hawkes(), statistic.NewHawkesState())

	if output := stream.Step(hawkesArrival(1, hawkesTestEpoch, 0)); output.Err != nil {
		t.Fatal(output.Err)
	}

	committed := stream.Project()

	if output := stream.Step(hawkesArrival(-1, hawkesTestEpoch-1, 0)); output.Err == nil {
		t.Fatal("regressed timestamp should fail")
	}

	if !stream.Project().Equal(committed) {
		t.Fatal("failed transition changed committed Hawkes state")
	}
}

func BenchmarkHawkes(b *testing.B) {
	stream := types.NewStream(Hawkes(), statistic.NewHawkesState())
	input := hawkesArrival(1, hawkesTestEpoch, 0)

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		input.Put(statistic.SymbolUnixSec, hawkesTestEpoch+float64(iteration))
		_ = stream.Step(input)
	}
}

func hawkesArrival(mark float64, sec float64, nsec float64) types.Frame {
	input := types.Frame{}
	input.Put(statistic.SymbolMark, mark)
	input.Put(statistic.SymbolUnixSec, sec)
	input.Put(statistic.SymbolUnixNsec, nsec)

	return input
}

func assertNumber(
	t *testing.T,
	frame types.Frame,
	symbol types.Symbol,
	want float64,
) {
	t.Helper()
	got := frame.MustGet(symbol)

	if got != want {
		t.Fatalf("symbol %d=%v; want %v", symbol, got, want)
	}
}
