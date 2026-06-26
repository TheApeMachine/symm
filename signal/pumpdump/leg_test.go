package pumpdump

import (
	"testing"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/testutil"
)

type legFrame struct {
	volume float64
	last   float64
}

/*
replayFrame seeds peers, inserts a touch book at the frame price, and measures
one ticker tick — persisting the measurement (leg anchors, exhaustion stamp)
into the tree so the next frame rebuilds leg context from the tree alone.
*/
func replayFrame(
	t testing.TB,
	signal *Signal,
	crossSection *market.CrossSection,
	symbol string,
	tick int,
	frame legFrame,
) {
	t.Helper()

	stamp := int64(tick+1) * int64(time.Second)
	seedPeers(t, crossSection, stamp, tick, 4000)
	insertBook(t, signal, symbol, stamp-1, frame.last-0.05, frame.last+0.05, 900, 900)

	measureTicker(t, signal, crossSection, tickerCase{
		symbol: symbol, stamp: stamp, volume: frame.volume, last: frame.last,
		bid: frame.last - 0.05, ask: frame.last + 0.05,
	})
}

/*
TestMultiLegSecondIgnitionSurvivesContaminatedBaseline replays leg-1 ignition →
consolidation → leg-2 ignition. The session baseline now includes leg 1's pump,
which would make leg 2 read "moderate". The leg anchor pins precursor to the
current consolidation range so leg-2 ignition is still detected.
*/
func TestMultiLegSecondIgnitionSurvivesContaminatedBaseline(t *testing.T) {
	signal := newTestSignal(t)
	crossSection := testutil.NewTestCrossSection(t)
	symbol := "MULTILEG/USD"

	frames := []legFrame{
		{volume: 1000, last: 100},
		{volume: 1100, last: 101},
		{volume: 4000, last: 140}, // leg 1 ignition
		{volume: 4100, last: 138}, // consolidation chop
		{volume: 4150, last: 139},
		{volume: 4200, last: 138},
		{volume: 4250, last: 139},
	}

	for tick, frame := range frames {
		replayFrame(t, signal, crossSection, symbol, tick, frame)
	}

	legTwo := tickerCase{
		symbol: symbol, stamp: int64(len(frames)+1) * int64(time.Second),
		volume: 7000, last: 175, bid: 174.95, ask: 175.05,
	}
	seedPeers(t, crossSection, legTwo.stamp, len(frames), 4000)
	insertBook(t, signal, symbol, legTwo.stamp-1, 174.95, 175.05, 900, 900)
	result := measureTicker(t, signal, crossSection, legTwo)

	if result == nil {
		t.Fatal("leg-2 Measure returned nil")
	}

	if categoryResult(result) != logic.CategoryIndex(logic.CategoryVerticalIgnition) {
		t.Fatalf("leg-2 category=%d, want vertical ignition (suppressed by contaminated baseline)", categoryResult(result))
	}

	if outputScore(result, "ignition") <= outputScore(result, "exhaustion") {
		t.Fatalf("ignition=%v exhaustion=%v: leg-2 ignition not dominant", outputScore(result, "ignition"), outputScore(result, "exhaustion"))
	}
}

/*
TestThinBookTrapDisqualifiesIgnition is the TITCOIN trap: a huge % move on tiny
USD volume, a wide spread, and a hollow book. Derived thin-book measures (bottom
dollar-volume rank, spread vs own median, depth vs own median) collapse ignition
toward no real pump while a structured vertical ignition stays intact.
*/
func TestThinBookTrapDisqualifiesIgnition(t *testing.T) {
	signal := newTestSignal(t)
	crossSection := testutil.NewTestCrossSection(t)
	symbol := "TITCOIN/USD"

	// Structured history: tight spreads, deep touch, normal volume.
	for tick := 0; tick < 6; tick++ {
		stamp := int64(tick+1) * int64(time.Second)
		insertPrior(t, signal, priorCase{
			key:         "h" + string(rune('a'+tick)),
			symbol:      symbol,
			stamp:       stamp,
			volume:      1000 + float64(tick)*10,
			last:        100 + float64(tick)*0.1,
			volumeDelta: 10,
			logReturn:   0.001,
			spread:      0.1,
			bookSpread:  0.1,
			tradeVolume: 10,
			touchDepth:  1800,
			rvol:        1,
			compression: 0,
		})
	}

	// Peers carry healthy dollar volume; the trap sits far below them.
	stamp := int64(7) * int64(time.Second)
	seedPeers(t, crossSection, stamp, 6, 5000)

	// Thin-book frame: huge % move, tiny volume, wide spread, hollow book.
	insertBook(t, signal, symbol, stamp-1, 90, 110, 1, 1)
	result := measureTicker(t, signal, crossSection, tickerCase{
		symbol: symbol, stamp: stamp, volume: 50, last: 400, bid: 90, ask: 110,
	})

	if result == nil {
		t.Fatal("thin-book Measure returned nil")
	}

	if got := datura.Peek[float64](result, "thinBook"); got <= 0 {
		t.Fatalf("thinBook=%v, want positive disqualification score", got)
	}

	if outputScore(result, "ignition") >= outputScore(result, "exhaustion") {
		t.Fatalf("ignition=%v exhaustion=%v: thin-book trap was not disqualified", outputScore(result, "ignition"), outputScore(result, "exhaustion"))
	}
}
