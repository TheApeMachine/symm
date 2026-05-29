package pumpdump

import (
	"sync"
	"testing"
	"time"

	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trade"
)

const minFastPumpSpike = 15.0 // matches default FastPumpVolumeRatio; test is config-independent

func TestPumpSymbolFastScaleDetection(t *testing.T) {
	state := NewPumpSymbol(asset.Pair{Wsname: "PUMP/EUR"})

	for range 12 {
		_, _ = state.fastVolumeBaseline.Next(0, 1)
		_, _ = state.mediumVolumeBaseline.Next(0, 1)
	}

	now := time.Unix(1_700_000_000, 0)

	for index := range 20 {
		state.FeedTradeVolume(now.Add(time.Duration(index)*100*time.Millisecond), 1, 1)
	}

	spike, regime, err := state.BestVolumeSpike()

	if err != nil {
		t.Fatalf("best volume spike: %v", err)
	}

	if regime != "fast_pump" {
		t.Fatalf("expected fast_pump regime, got %q spike=%v", regime, spike)
	}

	if spike < minFastPumpSpike {
		t.Fatalf("expected fast spike above %v, got %v", minFastPumpSpike, spike)
	}
}

func TestPumpSymbolBestVolumeSpikeUnwarmedBaselines(t *testing.T) {
	state := NewPumpSymbol(asset.Pair{Wsname: "PUMP/EUR"})
	now := time.Unix(1_700_000_000, 0)

	state.FeedTradeVolume(now, 100, 1)

	spike, regime, err := state.BestVolumeSpike()

	if err != nil {
		t.Fatalf("best volume spike: %v", err)
	}

	if spike != 0 || regime != "" {
		t.Fatalf("expected no spike before baselines warm, got spike=%v regime=%q", spike, regime)
	}
}

func TestPumpSymbolSlowRVOL(t *testing.T) {
	state := NewPumpSymbol(asset.Pair{Wsname: "PUMP/EUR"})
	state.SetMedianHourlyVolume(10)

	if !state.HourlyBaselineReady() {
		t.Fatal("expected hourly baseline ready after SetMedianHourlyVolume")
	}

	now := time.Unix(1_700_000_000, 0)

	for index := range 60 {
		state.FeedTradeVolume(now.Add(time.Duration(index)*time.Minute), 1, 1)
	}

	if state.SlowRVOL() < 5 {
		t.Fatalf("expected slow breakout RVOL, got %v", state.SlowRVOL())
	}
}

func TestPumpSymbolSlowRVOLBeforeWarm(t *testing.T) {
	state := NewPumpSymbol(asset.Pair{Wsname: "PUMP/EUR"})
	state.FeedTradeVolume(time.Unix(1_700_000_000, 0), 100, 1)

	if state.SlowRVOL() != 0 {
		t.Fatalf("expected zero RVOL before hourly baseline warm, got %v", state.SlowRVOL())
	}
}

func TestErrNoVolumeData(t *testing.T) {
	if ErrNoVolumeData.Error() != "no valid volume data" {
		t.Fatalf("unexpected ErrNoVolumeData message: %v", ErrNoVolumeData)
	}
}

func TestPumpSymbolConcurrentFeedAndMeasure(t *testing.T) {
	state := NewPumpSymbol(asset.Pair{Wsname: "PUMP/EUR"})
	state.SetMedianHourlyVolume(10)
	now := time.Unix(1_700_000_000, 0)
	var waiters sync.WaitGroup

	waiters.Go(func() {
		for index := range 128 {
			state.FeedTicker(market.TickerRow{
				Last:   1 + float64(index)*0.0001,
				Bid:    0.99,
				Ask:    1.01,
				Volume: 100,
			})
		}
	})
	waiters.Go(func() {
		for index := range 128 {
			state.FeedTrade(trade.Data{
				Side:      "buy",
				Qty:       1,
				Timestamp: now.Add(time.Duration(index) * time.Millisecond),
			})
		}
	})
	waiters.Go(func() {
		for range 128 {
			state.FeedBook(market.BookLevelsDelta{
				Bids: []market.BookLevel{{Price: 0.99, Volume: 2}},
				Asks: []market.BookLevel{{Price: 1.01, Volume: 1}},
			})
		}
	})
	waiters.Go(func() {
		for range 128 {
			state.BestVolumeSpike()
			state.SlowRVOL()
			state.Measure(2, "fast_pump")
		}
	})

	waiters.Wait()
}
