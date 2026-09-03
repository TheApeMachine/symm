package algo

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/statistic/hawkes"
	"github.com/theapemachine/symm/nomagique/types"
)

const hawkesTestEpoch = 1_786_099_200.0

func hawkesArrival(mark float64, sec float64, nsec float64) types.Frame {
	input := types.Frame{}
	input.Put(hawkes.SymbolMark, mark)
	input.Put(types.EventTimeSec, sec)
	input.Put(types.EventTimeNsec, nsec)

	return input
}

func assertNumber(t *testing.T, frame types.Frame, symbol types.Symbol, want float64) {
	t.Helper()
	got := frame.MustGet(symbol)

	if got != want {
		t.Fatalf("symbol %d=%v; want %v", symbol, got, want)
	}
}

/*
seedClusteredBurst drives count buy/sell events at a fixed cadence, evenly
split, dense enough to satisfy Fit's data-derived identifiability gate so
downstream tests can exercise fit-dependent metrics.
*/
func seedClusteredBurst(stream *types.Stream, count int) types.Frame {
	var output types.Frame

	for index := 0; index < count; index++ {
		mark := -1.0

		if index%2 == 0 {
			mark = 1
		}

		atNanos := int64(float64(index) * 0.3 * 1e9)
		sec := hawkesTestEpoch + float64(atNanos/1e9)
		nsec := float64(atNanos % 1e9)
		output = stream.Step(hawkesArrival(mark, sec, nsec))
	}

	return output
}

func TestHawkesPublishesEmpiricalCountsFromTheFirstEvent(t *testing.T) {
	stream := types.NewStream(Hawkes(), types.Frame{})
	output := stream.Step(hawkesArrival(1, hawkesTestEpoch, 0))

	if output.Err != nil {
		t.Fatal(output.Err)
	}

	assertNumber(t, output, hawkes.SymbolEventCount, 1)
	assertNumber(t, output, hawkes.SymbolEventCountBuy, 1)
	assertNumber(t, output, hawkes.SymbolEventCountSell, 0)
}

func TestHawkesModelStaysAbsentBeforeFitConverges(t *testing.T) {
	stream := types.NewStream(Hawkes(), types.Frame{})
	output := stream.Step(hawkesArrival(1, hawkesTestEpoch, 0))

	if output.Err != nil {
		t.Fatal(output.Err)
	}

	for _, symbol := range []types.Symbol{
		hawkes.SymbolConditionalIntensityBuy,
		hawkes.SymbolConditionalIntensitySell,
		hawkes.SymbolBackgroundRateBuy,
		hawkes.SymbolBackgroundRateSell,
		hawkes.SymbolSpectralRadius,
		hawkes.SymbolExcitationAmplitudeBB,
	} {
		if output.Has(symbol) {
			t.Fatalf("symbol %d must be absent before a fit converges", symbol)
		}
	}
}

func TestHawkesFitConvergesAndPublishesModelDependentMetrics(t *testing.T) {
	stream := types.NewStream(Hawkes(), types.Frame{})
	output := seedClusteredBurst(stream, 200)

	if output.Err != nil {
		t.Fatal(output.Err)
	}

	if !output.Has(hawkes.SymbolConditionalIntensityBuy) {
		t.Fatal("expected a converged fit to publish conditional_intensity:buy")
	}

	if output.MustGet(hawkes.SymbolConditionalIntensityBuy) <= 0 {
		t.Fatal("expected a positive fitted buy intensity")
	}

	if output.MustGet(hawkes.SymbolConditionalIntensitySell) <= 0 {
		t.Fatal("expected a positive fitted sell intensity")
	}

	spectralRadius := output.MustGet(hawkes.SymbolSpectralRadius)

	if spectralRadius < 0 || spectralRadius >= 1 {
		t.Fatalf("expected a subcritical spectral radius, got %v", spectralRadius)
	}
}

func TestHawkesRejectsTimeRegressionTransactionally(t *testing.T) {
	stream := types.NewStream(Hawkes(), types.Frame{})

	if output := stream.Step(hawkesArrival(1, hawkesTestEpoch, 0)); output.Err != nil {
		t.Fatal(output.Err)
	}

	committed := stream.Project()

	if output := stream.Step(hawkesArrival(-1, hawkesTestEpoch-1, 0)); output.Err == nil {
		t.Fatal("regressed timestamp should fail")
	}

	projected := stream.Project()

	if !projected.Equal(&committed) {
		t.Fatal("failed transition changed committed Hawkes state")
	}
}

func BenchmarkHawkes(b *testing.B) {
	stream := types.NewStream(Hawkes(), types.Frame{})
	input := hawkesArrival(1, hawkesTestEpoch, 0)

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		input.Put(types.EventTimeSec, hawkesTestEpoch+float64(iteration))
		_ = stream.Step(input)
	}
}
