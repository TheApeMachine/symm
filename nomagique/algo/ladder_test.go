package algo

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique"
)

const (
	ladderTestHalflife = 60.0
	ladderTestEpoch    = ignitionTestEpoch
)

func ladderObservationForTest(
	bidDepth float64,
	askDepth float64,
	spread float64,
	bidDelta float64,
	askDelta float64,
	unixSec float64,
) nomagique.Frame {
	input := nomagique.Frame{}
	input.Put(SymbolLadderHalflife, ladderTestHalflife)
	input.Put(SymbolLadderBidDepth, bidDepth)
	input.Put(SymbolLadderAskDepth, askDepth)
	input.Put(SymbolLadderSpread, spread)
	input.Put(SymbolLadderBidDelta, bidDelta)
	input.Put(SymbolLadderAskDelta, askDelta)
	input.Put(SymbolUnixSec, unixSec)
	input.Put(SymbolUnixNsec, 0)

	return input
}

func measureLadder(
	t *testing.T,
	stream *nomagique.Stream,
	input nomagique.Frame,
) nomagique.Frame {
	t.Helper()
	output, err := stream.Step(input)

	if err != nil {
		t.Fatal(err)
	}

	return output
}

func assertAbsent(
	t *testing.T,
	frame nomagique.Frame,
	symbol nomagique.Symbol,
) {
	t.Helper()

	if _, found := frame.Get(symbol); found {
		t.Fatalf("symbol %d present; want absent", symbol)
	}
}

func TestLadderSeedsBaselineWithFirstObservation(t *testing.T) {
	stream := nomagique.NewStream(Ladder, nomagique.Frame{})
	first := measureLadder(t, stream, ladderObservationForTest(
		100, 100, 10, 0, 0, ladderTestEpoch,
	))

	assertNumber(t, first, SymbolLadderSpreadBaseline, 10)
	assertNumber(t, first, SymbolLadderCompression, 0)
	assertNumber(t, first, SymbolLadderImbalance, 0)
	assertNumber(t, first, SymbolLadderReady, 1)
	assertNumber(t, first, SymbolLadderMaturity, 0.5)
	assertAbsent(t, first, SymbolLadderBidDepletion)
	assertAbsent(t, first, SymbolLadderAskDepletion)
}

func TestLadderHalflifeAdaptsByEventTime(t *testing.T) {
	stream := nomagique.NewStream(Ladder, nomagique.Frame{})
	measureLadder(t, stream, ladderObservationForTest(
		100, 100, 10, 0, 0, ladderTestEpoch,
	))

	// One halflife later the spread baseline has moved halfway from 10
	// toward 1, so compression scores the pinch against the decayed norm.
	halflifeLater := measureLadder(t, stream, ladderObservationForTest(
		100, 100, 1, 0, 0, ladderTestEpoch+ladderTestHalflife,
	))

	assertAlmostEqual(t,
		halflifeLater.MustGet(SymbolLadderSpreadBaseline), 5.5, 1e-9)
	assertAlmostEqual(t,
		halflifeLater.MustGet(SymbolLadderCompression), 1-1/5.5, 1e-9)

	// A dense pass an instant later barely moves the baseline: the budget
	// belongs to the event gaps, not to a count.
	dense := measureLadder(t, stream, ladderObservationForTest(
		100, 100, 1, 0, 0, ladderTestEpoch+ladderTestHalflife+0.001,
	))

	assertAlmostEqual(t,
		dense.MustGet(SymbolLadderSpreadBaseline), 5.5, 1e-3)

	// A sparse gap of ten halflives adopts the new regime almost fully:
	// the residual is the old baseline times two to the minus ten.
	sparse := measureLadder(t, stream, ladderObservationForTest(
		100, 100, 1, 0, 0, ladderTestEpoch+ladderTestHalflife*11,
	))

	assertAlmostEqual(t,
		sparse.MustGet(SymbolLadderSpreadBaseline), 1, 5.5/(1<<10)+1e-9)
}

func TestLadderScoresSideDepletionAgainstOwnBaseline(t *testing.T) {
	stream := nomagique.NewStream(Ladder, nomagique.Frame{})

	// A quiet seeding pass, then an ask-side depletion twice the size of
	// the first one the ladder ever saw.
	first := measureLadder(t, stream, ladderObservationForTest(
		100, 100, 10, 0, -25, ladderTestEpoch,
	))
	assertNumber(t, first, SymbolLadderAskDepletion, 0.5)

	later := measureLadder(t, stream, ladderObservationForTest(
		100, 40, 10, 0, -50, ladderTestEpoch+ladderTestHalflife*10,
	))

	// Ten halflives later the baseline adopted the new magnitude, so the
	// same-sized event scores near one half against its own norm.
	assertAlmostEqual(t,
		later.MustGet(SymbolLadderAskDepletion), 0.5, 1e-3)
	assertAbsent(t, later, SymbolLadderBidDepletion)
	assertAbsent(t, later, SymbolLadderBidReplenish)

	// Bid replenishment is tracked independently of ask depletion.
	stacked := measureLadder(t, stream, ladderObservationForTest(
		160, 40, 10, 60, 0, ladderTestEpoch+ladderTestHalflife*20,
	))

	if _, found := stacked.Get(SymbolLadderBidReplenish); !found {
		t.Fatal("bid replenishment absent after a stacking pass")
	}
}

func TestLadderImbalanceRequiresBothSides(t *testing.T) {
	stream := nomagique.NewStream(Ladder, nomagique.Frame{})

	output := measureLadder(t, stream, ladderObservationForTest(
		0, 100, 10, 0, 0, ladderTestEpoch,
	))

	assertAbsent(t, output, SymbolLadderImbalance)

	balanced := measureLadder(t, stream, ladderObservationForTest(
		math.E*100, 100, 10, 0, 0, ladderTestEpoch+1,
	))

	assertAlmostEqual(t, balanced.MustGet(SymbolLadderImbalance), 1, 1e-9)
}

func TestLadderRejectsInvalidObservationTransactionally(t *testing.T) {
	stream := nomagique.NewStream(Ladder, nomagique.Frame{})
	measureLadder(t, stream, ladderObservationForTest(
		100, 100, 10, 0, 0, ladderTestEpoch,
	))

	// Time regression must fail without touching retained baselines.
	if _, err := stream.Step(ladderObservationForTest(
		100, 100, 10, 0, 0, ladderTestEpoch-1,
	)); err == nil {
		t.Fatal("time regression accepted")
	}

	// Negative depth must fail the same way.
	if _, err := stream.Step(ladderObservationForTest(
		-1, 100, 10, 0, 0, ladderTestEpoch+1,
	)); err == nil {
		t.Fatal("negative depth accepted")
	}

	// A missing halflife must fail before any state change.
	broken := ladderObservationForTest(100, 100, 10, 0, 0, ladderTestEpoch+2)
	broken.Delete(SymbolLadderHalflife)

	if _, err := stream.Step(broken); err == nil {
		t.Fatal("missing halflife accepted")
	}

	survived := measureLadder(t, stream, ladderObservationForTest(
		100, 100, 10, 0, 0, ladderTestEpoch+3,
	))
	assertNumber(t, survived, SymbolLadderSpreadBaseline, 10)
}

func BenchmarkLadder(b *testing.B) {
	stream := nomagique.NewStream(Ladder, nomagique.Frame{})
	b.ReportAllocs()

	for index := 0; index < b.N; index++ {
		input := ladderObservationForTest(
			100, 100, 10, 1, -1, ladderTestEpoch+float64(index),
		)

		if _, err := stream.Step(input); err != nil {
			b.Fatal(err)
		}
	}
}
