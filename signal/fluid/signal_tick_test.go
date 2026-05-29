package fluid

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trade"
)

func TestFluidTickRegistersSymbols(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewFluid(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	subscriptions := signal.broadcasts["subscriptions"].Subscribe("test:fluid-subs", 8)
	startFluidTick(t, signal)

	pool.CreateBroadcastGroup("symbols", 0).Send(&qpool.QValue[any]{
		Value: map[string]*asset.Pair{
			"ALT/EUR": {Wsname: "ALT/EUR", Quote: config.System.QuoteCurrency},
		},
	})

	deadline := time.Now().Add(time.Second)

	for time.Now().Before(deadline) {
		if loadFluidSymbol(signal, "ALT/EUR") != nil {
			break
		}

		time.Sleep(time.Millisecond)
	}

	if loadFluidSymbol(signal, "ALT/EUR") == nil {
		t.Fatal("expected symbol state after symbols tick")
	}

	select {
	case value := <-subscriptions.Incoming:
		symbols, ok := value.Value.([]string)

		if !ok || len(symbols) == 0 {
			t.Fatalf("expected subscription flush, got %v", value.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscription flush")
	}
}

func TestFluidTickAppliesTicker(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewFluid(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	storeFluidSymbol(signal, "ALT/EUR", NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"}))
	startFluidTick(t, signal)

	pool.CreateBroadcastGroup("tick", 0).Send(&qpool.QValue[any]{
		Value: market.TickerRow{
			Symbol:    "ALT/EUR",
			Last:      10.5,
			Bid:       10.4,
			Ask:       10.6,
			Volume:    5000,
			ChangePct: 2.2,
		},
	})

	deadline := time.Now().Add(time.Second)

	for time.Now().Before(deadline) {
		state := loadFluidSymbol(signal, "ALT/EUR")

		if state == nil {
			time.Sleep(time.Millisecond)
			continue
		}

		row := state.wireRow()

		if row != nil && row["change_pct"] == 2.2 {
			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatal("expected ticker row to update symbol state")
}

func TestFluidPublishPulseAfterTrade(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewFluid(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	state := NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"})
	storeFluidSymbol(signal, "ALT/EUR", state)
	markFluidRequested(signal, "ALT/EUR")
	seedFluidSymbol(state)

	measurements := signal.broadcasts["measurements"].Subscribe("test:fluid-trade", 8)
	startFluidTick(t, signal)

	pool.CreateBroadcastGroup("trade", 0).Send(&qpool.QValue[any]{
		Value: trade.Data{
			Symbol:    "ALT/EUR",
			Side:      "buy",
			Qty:       0.5,
			Price:     10,
			Timestamp: time.Now(),
		},
	})

	select {
	case value := <-measurements.Incoming:
		measurement, ok := value.Value.(engine.Measurement)

		if !ok || measurement.Source != fluidSource {
			t.Fatalf("expected fluid measurement, got %v", value.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for fluid measurement after trade tick")
	}
}

func TestFluidTickIgnoresUnknownTrade(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewFluid(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	measurements := signal.broadcasts["measurements"].Subscribe("test:fluid-unknown", 8)
	startFluidTick(t, signal)

	pool.CreateBroadcastGroup("trade", 0).Send(&qpool.QValue[any]{
		Value: trade.Data{
			Symbol:    "NOPE/EUR",
			Side:      "buy",
			Qty:       0.5,
			Price:     10,
			Timestamp: time.Now(),
		},
	})

	select {
	case value := <-measurements.Incoming:
		t.Fatalf("expected no measurement for unknown trade, got %v", value.Value)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestFluidBookRequestsSubscription(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewFluid(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	storeFluidSymbol(signal, "ALT/EUR", NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"}))

	subscriptions := signal.broadcasts["subscriptions"].Subscribe("test:fluid-book-sub", 8)
	startFluidTick(t, signal)

	pool.CreateBroadcastGroup("book", 0).Send(&qpool.QValue[any]{
		Value: market.BookLevelsDelta{
			Symbol: "ALT/EUR",
			BidOK:  true,
			AskOK:  true,
			Bids:   []market.BookLevel{{Price: 10, Volume: 50}},
			Asks:   []market.BookLevel{{Price: 10.02, Volume: 40}},
		},
	})

	select {
	case value := <-subscriptions.Incoming:
		symbols, ok := value.Value.([]string)

		if !ok || len(symbols) != 1 || symbols[0] != "ALT/EUR" {
			t.Fatalf("expected subscription for ALT/EUR, got %v", value.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for book-driven subscription")
	}
}

func TestFluidTickAcceptsFeedback(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewFluid(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	storeFluidSymbol(signal, "ALT/EUR", NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"}))
	markFluidRequested(signal, "ALT/EUR")

	startFluidTick(t, signal)

	pool.CreateBroadcastGroup("feedback", 0).Send(&qpool.QValue[any]{
		Value: engine.PredictionFeedback{
			Source:          fluidSource,
			Symbol:          "ALT/EUR",
			PredictedReturn: 0.02,
			ActualReturn:    -0.01,
		},
	})

	time.Sleep(20 * time.Millisecond)
}

func TestFluidPendingBatchRespectsMaxScan(t *testing.T) {
	originalMaxScan := config.System.MaxScanSymbols
	config.System.MaxScanSymbols = 2
	t.Cleanup(func() { config.System.MaxScanSymbols = originalMaxScan })

	signal := &Fluid{}
	signal.requested.Store("A/EUR", struct{}{})
	signal.requested.Store("B/EUR", struct{}{})
	signal.queuePending("C/EUR")

	if batch := signal.pendingBatch(); batch != nil {
		t.Fatalf("expected nil batch at max scan, got %v", batch)
	}
}

func TestFluidPublishPulseMarksRequested(t *testing.T) {
	originalMaxScan := config.System.MaxScanSymbols
	originalBatch := config.System.SubscribeBatch
	config.System.MaxScanSymbols = 8
	config.System.SubscribeBatch = 4
	t.Cleanup(func() {
		config.System.MaxScanSymbols = originalMaxScan
		config.System.SubscribeBatch = originalBatch
	})

	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewFluid(ctx, pool)
	signal.queuePending("ALT/EUR")

	subscriptions := signal.broadcasts["subscriptions"].Subscribe("test:fluid-pulse", 8)
	signal.publishPulse()

	select {
	case value := <-subscriptions.Incoming:
		symbols, ok := value.Value.([]string)

		if !ok || len(symbols) != 1 || symbols[0] != "ALT/EUR" {
			t.Fatalf("expected pending flush, got %v", value.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for publishPulse subscription flush")
	}

	if _, ok := signal.requested.Load("ALT/EUR"); !ok {
		t.Fatal("expected publishPulse to mark symbol requested")
	}
}
