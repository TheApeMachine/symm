package fluid

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
)

func testFluidWithSymbol(t *testing.T) (*Fluid, *FluidSymbol) {
	t.Helper()

	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewFluid(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	state := NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR", Quote: "EUR"})
	storeFluidSymbol(signal, "ALT/EUR", state)
	markFluidRequested(signal, "ALT/EUR")

	loaded := loadFluidSymbol(signal, "ALT/EUR")

	if loaded == nil {
		t.Fatal("expected fluid symbol state")
	}

	return signal, loaded
}

func seedFluidSymbol(state *FluidSymbol) {
	state.bids = []market.BookLevel{{Price: 10, Volume: 80}}
	state.asks = []market.BookLevel{{Price: 10.01, Volume: 20}}
	state.spreadBPS = 10
	state.buyPressure, _ = state.pressure.Next(0, 0.8)

	for range 8 {
		_, _ = state.score.Push(0.7, 0.8)
	}
}

func TestFluidSourceStartState(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewFluid(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	if signal.Source() != fluidSource {
		t.Fatalf("expected source %q, got %q", fluidSource, signal.Source())
	}

	if signal.State() != engine.READY {
		t.Fatalf("expected READY state, got %v", signal.State())
	}

	if signal.Start() != nil {
		t.Fatal("expected Start to succeed")
	}
}

func TestFluidMeasure(t *testing.T) {
	signal, state := testFluidWithSymbol(t)
	seedFluidSymbol(state)

	found := false

	for measurement := range signal.Measure() {
		found = true

		if measurement.Source != fluidSource || measurement.Confidence <= 0 {
			t.Fatalf("unexpected measurement: %+v", measurement)
		}

		if measurement.Type != engine.Flow {
			t.Fatalf("expected flow type, got %+v", measurement.Type)
		}
	}

	if !found {
		t.Fatal("expected at least one measurement")
	}
}

func TestFluidMeasureSkipsUnrequestedSymbols(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewFluid(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	state := NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"})
	state.changePct = 3
	state.volume = 1000
	storeFluidSymbol(signal, "ALT/EUR", state)

	for range signal.Measure() {
		t.Fatal("expected no measurements for unrequested symbols")
	}
}

func TestFluidFeedbackLowersForecastScale(t *testing.T) {
	signal, state := testFluidWithSymbol(t)
	seedFluidSymbol(state)

	before := state.forecast.Scale()

	signal.Feedback(engine.PredictionFeedback{
		Source:          fluidSource,
		Symbol:          "ALT/EUR",
		PredictedReturn: 0.02,
		ActualReturn:    -0.01,
	})

	after := state.forecast.Scale()

	if after >= before {
		t.Fatalf("expected losing feedback to lower scale, before=%v after=%v", before, after)
	}
}

func TestFluidFeedbackIgnoresForeignSource(t *testing.T) {
	signal, state := testFluidWithSymbol(t)
	before := state.forecast.Scale()

	signal.Feedback(engine.PredictionFeedback{
		Source:          "pumpdump",
		Symbol:          "ALT/EUR",
		PredictedReturn: 0.02,
		ActualReturn:    -0.01,
	})

	if state.forecast.Scale() != before {
		t.Fatal("expected foreign feedback to be ignored")
	}
}

func TestFluidPublishFieldRows(t *testing.T) {
	signal, state := testFluidWithSymbol(t)
	state.changePct = 2.5
	state.volume = 1200

	ui := signal.broadcasts["ui"].Subscribe("test:fluid-ui", 8)

	signal.publishFieldRows()

	sawSnapshot := false

	for range 2 {
		select {
		case value := <-ui.Incoming:
			payload, ok := value.Value.(map[string]any)

			if ok && payload["event"] == "field_snapshot" {
				sawSnapshot = true
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for field rows")
		}
	}

	if !sawSnapshot {
		t.Fatal("expected field_snapshot after field_row")
	}
}

func BenchmarkFluidPublishMeasurements(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	b.Cleanup(func() { pool.Close() })

	signal := NewFluid(ctx, pool)
	b.Cleanup(func() { _ = signal.Close() })

	state := NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"})
	state.changePct = 2
	state.volume = 500
	seedFluidSymbol(state)
	storeFluidSymbol(signal, "ALT/EUR", state)
	markFluidRequested(signal, "ALT/EUR")

	b.ReportAllocs()

	for b.Loop() {
		signal.publishMeasurements()
	}
}
