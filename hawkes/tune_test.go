package hawkes

import (
	"math"
	"testing"
	"time"

	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric"
)

func TestMinFitEventsScalesWithArrivalCount(t *testing.T) {
	low := minFitEventsFor(16)
	high := minFitEventsFor(64)

	if high <= low {
		t.Fatalf("expected higher event floor for denser streams, low=%d high=%d", low, high)
	}

	if low < bivariateParamCount*2 {
		t.Fatalf("expected identifiability floor, got %d", low)
	}
}

func TestBranchCeilingTightensWithSampleSize(t *testing.T) {
	sparse := branchCeilingFor(16)
	dense := branchCeilingFor(256)

	if dense <= sparse {
		t.Fatalf("expected tighter ceiling with more events, sparse=%v dense=%v", sparse, dense)
	}

	if sparse >= criticalBranch {
		t.Fatalf("expected subcritical ceiling, got %v", sparse)
	}
}

func TestTradeWindowDurationScalesWithGapAndCount(t *testing.T) {
	short := tradeWindowDuration(0.05, 16, minFitEventsFor(16))
	long := tradeWindowDuration(0.2, 64, minFitEventsFor(64))

	if long <= short {
		t.Fatalf("expected longer window for slower or denser streams, short=%v long=%v", short, long)
	}
}

func TestLogSpaceCoversBetaRange(t *testing.T) {
	values := numeric.LogSpace(1, 10, 5)

	if len(values) != 5 {
		t.Fatalf("expected five beta candidates, got %d", len(values))
	}

	if values[0] >= values[len(values)-1] {
		t.Fatal("expected ascending log-spaced values")
	}

	if math.Abs(values[0]-1) > 1e-9 {
		t.Fatalf("expected lower beta bound near 1, got %v", values[0])
	}
}

func TestFitContextFromTicksAdaptsWindow(t *testing.T) {
	start := time.Unix(0, 0)
	buyEvents := burstEvents(start, 16, 40*time.Millisecond)
	sellEvents := sparseSellEvents(start.Add(-time.Second), 6)
	ticks := make([]market.TradeTick, 0, len(buyEvents)+len(sellEvents))

	for _, eventTime := range buyEvents {
		ticks = append(ticks, market.TradeTick{Side: "buy", Timestamp: eventTime})
	}

	for _, eventTime := range sellEvents {
		ticks = append(ticks, market.TradeTick{Side: "sell", Timestamp: eventTime})
	}

	horizon := buyEvents[len(buyEvents)-1].Add(50 * time.Millisecond)
	context, _, _, ok := fitContextFromTicks(ticks, time.Time{}, horizon)

	if !ok {
		t.Fatal("expected adaptive fit context from ticks")
	}

	if context.TradeWindow <= 0 {
		t.Fatal("expected positive adaptive trade window")
	}
}
