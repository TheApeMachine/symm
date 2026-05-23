package trader

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	kbook "github.com/theapemachine/symm/kraken/book"
	"github.com/theapemachine/symm/kraken/client"
	kticker "github.com/theapemachine/symm/kraken/ticker"
	"github.com/theapemachine/symm/kraken/trades"
	"github.com/theapemachine/symm/replay"
	"github.com/theapemachine/symm/work"
)

func TestReplayPipelineFeedsTraderQuotes(t *testing.T) {
	frames, err := replay.LoadFrames(filepath.Join("..", "replay", "fixtures", "sample.jsonl"))

	if err != nil {
		t.Fatalf("load frames: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	publicClient := client.NewPublicClient(ctx, client.WithReplay(frames, 0))

	if err := publicClient.Connect(); err != nil {
		t.Fatalf("connect replay client: %v", err)
	}

	symbols := []string{"BTC/EUR", "PUMP/EUR"}

	bookObserver, err := kbook.New(ctx, publicClient, symbols)

	if err != nil {
		t.Fatalf("book observer: %v", err)
	}

	tradesObserver, err := trades.New(ctx, publicClient, symbols)

	if err != nil {
		t.Fatalf("trades observer: %v", err)
	}

	tickerObserver, err := kticker.New(ctx, publicClient, symbols)

	if err != nil {
		t.Fatalf("ticker observer: %v", err)
	}

	publicClient.StartReplay()

	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		if last, ok := tickerObserver.Last("PUMP/EUR"); ok && last > 0 {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	last, ok := tickerObserver.Last("PUMP/EUR")

	if !ok || last <= 0 {
		t.Fatal("expected replay ticker price for PUMP/EUR")
	}

	pressure, pressureOK := tradesObserver.BuyPressure("PUMP/EUR")

	if !pressureOK || pressure <= 0 {
		t.Fatalf("expected replay buy pressure, got ok=%v pressure=%v", pressureOK, pressure)
	}

	stub := &stubSignal{
		measurements: []engine.Measurement{{
			Type:       engine.Pump,
			Regime:     "pump",
			Reason:     "actual_pump",
			Pairs:      []asset.Pair{{Wsname: "PUMP/EUR", Base: "PUMP", Quote: "EUR"}},
			Confidence: 0.9,
		}},
	}

	wallet := NewWallet(PaperWallet, "EUR", config.DefaultWalletEUR, config.System.TakerFeePct)
	pool := work.NewPool(ctx)

	crypto, err := NewCrypto(
		ctx,
		pool,
		wallet,
		tickerObserver,
		NoopPublisher(),
		stub,
	)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	crypto.SetEngineStats(stubEngineStats{
		tickerReady: func() int { return len(symbols) },
		symbolTotal: func() int { return len(symbols) },
	})
	crypto.pulseSeq.Store(int64(config.System.MinWarmPulses))

	decideForEntry(crypto, stub.measurements)

	hold, held := crypto.holds["PUMP/EUR"]

	if !held {
		t.Fatal("expected paper entry from replay-fed quotes")
	}

	if hold.entryPrice <= 0 {
		t.Fatalf("expected positive entry fill, got %v", hold.entryPrice)
	}

	_ = bookObserver
}

func TestCryptoParallelDrainUsesPool(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	first := &stubSignal{measurements: []engine.Measurement{{Type: engine.Pump, Confidence: 0.5}}}
	second := &stubSignal{measurements: []engine.Measurement{{Type: engine.Momentum, Confidence: 0.4}}}
	pool := work.NewPool(ctx)

	crypto, err := NewCrypto(
		context.Background(),
		pool,
		NewWallet(PaperWallet, "EUR", 200, 0.26),
		stubPrices{"PUMP/EUR": 1},
		NoopPublisher(),
		first,
		second,
	)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	crypto.pulseSeq.Store(int64(config.System.MinWarmPulses))

	batch := crypto.drainMeasurements()

	if len(batch) != 2 {
		t.Fatalf("expected two pooled measurements, got %d", len(batch))
	}
}
