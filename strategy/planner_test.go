package strategy

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/types"
)

/*
stubSignal returns a fixed measurement batch so Update can be exercised without
live market feeds or algorithm readiness.
*/
type stubSignal struct {
	measurements []*types.Measurement
}

/*
Measure returns the stubbed batch synchronously, matching the live Signal
contract after the channel-returning Measure API was retired.
*/
func (signal stubSignal) Measure(*types.Thesis) []*types.Measurement {
	return signal.measurements
}

/*
TestPlannerUpdateCollectsEverySignal ensures Update returns after every signal
finishes. A fan-in that ranges until close without an independent closer hangs
here once all sends complete.
*/
func TestPlannerUpdateCollectsEverySignal(t *testing.T) {
	t.Parallel()

	planner := decideFixture.Planner(
		stubSignal{measurements: []*types.Measurement{{Symbol: "AAA/USD"}}},
		stubSignal{measurements: []*types.Measurement{{Symbol: "BBB/USD"}}},
		stubSignal{measurements: []*types.Measurement{{Symbol: "CCC/USD"}}},
	)

	done := make(chan *types.Thesis, 1)

	go func() {
		done <- planner.Update(&types.MarketFrame{}, 1)
	}()

	select {
	case thesis := <-done:
		if len(thesis.Measurements) != 3 {
			t.Fatalf(
				"expected 3 measurements, got %d",
				len(thesis.Measurements),
			)
		}

		if thesis.Tick != 1 {
			t.Fatalf("expected tick 1, got %d", thesis.Tick)
		}
	case <-time.After(time.Second):
		t.Fatal("planner.Update deadlocked collecting signal results")
	}
}

/*
BenchmarkPlannerUpdate measures concurrent signal collection cost for one
immutable market cut so fan-in regressions show up in allocation pressure.
*/
func BenchmarkPlannerUpdate(b *testing.B) {
	planner := decideFixture.Planner(
		stubSignal{measurements: []*types.Measurement{{Symbol: "AAA/USD"}}},
		stubSignal{measurements: []*types.Measurement{{Symbol: "BBB/USD"}}},
		stubSignal{measurements: []*types.Measurement{{Symbol: "CCC/USD"}}},
	)
	frame := &types.MarketFrame{}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		planner.Update(frame, 1)
	}
}
