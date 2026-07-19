package strategy

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
populatedFrame carries one ticker row so FrameInterest is ticker-only and the
planner fans the cut only to ticker-interested signals.
*/
func populatedFrame() *types.MarketFrame {
	return &types.MarketFrame{
		At:      time.Unix(1, 0).UTC(),
		Tickers: []kraken.TickerData{{Symbol: "AAA/USD"}},
	}
}

/*
stubSignal returns a fixed measurement batch and advertises an explicit stream
interest so Update can be exercised without live market feeds.
*/
type stubSignal struct {
	measurements []*types.Measurement
	interest     types.StreamInterest
	calls        atomic.Int64
}

/*
Interest reports the feeds this stub consumes. Zero defaults to StreamAll so
legacy fan-in tests keep measuring every worker.
*/
func (signal *stubSignal) Interest() types.StreamInterest {
	if signal.interest == 0 {
		return types.StreamAll
	}

	return signal.interest
}

/*
Measure returns the stubbed batch synchronously and counts invocations for
interest-filter assertions.
*/
func (signal *stubSignal) Measure(*types.Thesis) []*types.Measurement {
	signal.calls.Add(1)

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
		&stubSignal{measurements: []*types.Measurement{{Symbol: "AAA/USD"}}},
		&stubSignal{measurements: []*types.Measurement{{Symbol: "BBB/USD"}}},
		&stubSignal{measurements: []*types.Measurement{{Symbol: "CCC/USD"}}},
	)

	done := make(chan *types.Thesis, 1)

	go func() {
		done <- planner.Update(nil, populatedFrame(), 1)
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
TestPlannerUpdateHonorsSignalInterest proves a ticker-only cut runs ticker
workers and skips trade/book workers, while an empty cut still skips everyone.
*/
func TestPlannerUpdateHonorsSignalInterest(t *testing.T) {
	t.Parallel()

	ticker := &stubSignal{
		interest:     types.StreamTicker,
		measurements: []*types.Measurement{{Symbol: "AAA/USD", Source: "ticker"}},
	}
	trade := &stubSignal{
		interest:     types.StreamTrade,
		measurements: []*types.Measurement{{Symbol: "AAA/USD", Source: "trade"}},
	}
	book := &stubSignal{
		interest:     types.StreamBook,
		measurements: []*types.Measurement{{Symbol: "AAA/USD", Source: "book"}},
	}
	planner := decideFixture.Planner(ticker, trade, book)

	thesis := planner.Update(nil, populatedFrame(), 1)

	if ticker.calls.Load() != 1 {
		t.Fatalf("want ticker worker measured once, got %d", ticker.calls.Load())
	}

	if trade.calls.Load() != 0 || book.calls.Load() != 0 {
		t.Fatalf(
			"want trade/book skipped on ticker cut, got trade=%d book=%d",
			trade.calls.Load(), book.calls.Load(),
		)
	}

	if len(thesis.Measurements) != 1 {
		t.Fatalf("want one ticker measurement, got %d", len(thesis.Measurements))
	}

	empty := planner.Update(nil, &types.MarketFrame{}, 2)

	if ticker.calls.Load() != 1 || trade.calls.Load() != 0 || book.calls.Load() != 0 {
		t.Fatalf(
			"empty frame must not measure: ticker=%d trade=%d book=%d",
			ticker.calls.Load(), trade.calls.Load(), book.calls.Load(),
		)
	}

	if len(empty.Measurements) != 0 {
		t.Fatalf("empty frame must leave measurements empty, got %d", len(empty.Measurements))
	}
}

/*
TestPlannerUpdateMarksFailedMeasureIncomplete ensures a panicking signal fails
the cut without publishing measurements or killing its worker for later cuts.
*/
func TestPlannerUpdateMarksFailedMeasureIncomplete(t *testing.T) {
	t.Parallel()

	panicSignal := &panicStub{interest: types.StreamTicker}
	okSignal := &stubSignal{
		interest:     types.StreamTicker,
		measurements: []*types.Measurement{{Symbol: "AAA/USD", Source: "ok"}},
	}
	planner := decideFixture.Planner(panicSignal, okSignal)

	thesis := planner.Update(nil, populatedFrame(), 1)

	if !thesis.Incomplete() {
		t.Fatal("expected incomplete cut after signal panic")
	}

	if len(thesis.Measurements) != 0 {
		t.Fatalf("failed cut must clear measurements, got %d", len(thesis.Measurements))
	}

	retry := planner.Update(nil, populatedFrame(), 2)

	if !retry.Incomplete() {
		t.Fatal("panicking worker must stay alive and keep failing the cut")
	}
}

/*
panicStub panics from Measure so Update's recover path can be exercised.
*/
type panicStub struct {
	interest types.StreamInterest
}

/*
Interest advertises ticker interest for the ticker-only populated frame.
*/
func (signal *panicStub) Interest() types.StreamInterest {
	return signal.interest
}

/*
Measure always panics so the planner recover path is the unit under test.
*/
func (signal *panicStub) Measure(*types.Thesis) []*types.Measurement {
	panic("measure boom")
}

/*
BenchmarkPlannerUpdate measures concurrent signal collection cost for one
immutable market cut so fan-in regressions show up in allocation pressure.
*/
func BenchmarkPlannerUpdate(b *testing.B) {
	planner := decideFixture.Planner(
		&stubSignal{measurements: []*types.Measurement{{Symbol: "AAA/USD"}}},
		&stubSignal{measurements: []*types.Measurement{{Symbol: "BBB/USD"}}},
		&stubSignal{measurements: []*types.Measurement{{Symbol: "CCC/USD"}}},
	)
	frame := populatedFrame()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		planner.Update(nil, frame, 1)
	}
}
