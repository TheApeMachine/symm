package fluid

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
)

func withMinFillToCancelRatio(ratio float64, run func()) {
	previous := config.System.MinFillToCancelRatio
	config.System.MinFillToCancelRatio = ratio

	defer func() {
		config.System.MinFillToCancelRatio = previous
	}()

	run()
}

func TestSideChangeFlux(t *testing.T) {
	previous := []market.BookLevel{
		{Price: 100, Volume: 10},
		{Price: 99.5, Volume: 5},
	}

	addition := sideChangeFlux(previous, []market.BookLevel{
		{Price: 100, Volume: 10},
		{Price: 99.5, Volume: 5},
		{Price: 99, Volume: 3},
	})

	if addition != 3 {
		t.Fatalf("expected addition flux 3, got %v", addition)
	}

	volumeChange := sideChangeFlux(previous, []market.BookLevel{
		{Price: 100, Volume: 20},
		{Price: 99.5, Volume: 5},
	})

	if volumeChange != 10 {
		t.Fatalf("expected volume change flux 10, got %v", volumeChange)
	}

	removal := sideChangeFlux(previous, []market.BookLevel{
		{Price: 100, Volume: 10},
	})

	if removal != 5 {
		t.Fatalf("expected removal flux 5, got %v", removal)
	}
}

func TestBookFluxTrustworthy(t *testing.T) {
	state := NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"})
	now := time.Unix(1_700_000_000, 0)

	if !state.bookFluxTrustworthy() {
		t.Fatal("expected trustworthy with no book flux yet")
	}

	state.prevBids = []market.BookLevel{{Price: 10, Volume: 50}}
	state.prevAsks = []market.BookLevel{{Price: 10.01, Volume: 50}}
	state.FeedBook(market.BookLevelsDelta{
		BidOK: true,
		AskOK: true,
		Bids:  []market.BookLevel{{Price: 10, Volume: 100}},
		Asks:  []market.BookLevel{{Price: 10.01, Volume: 50}},
	})

	if state.bookFluxWindow.Sum() <= 0 {
		t.Fatalf("expected book flux after update, got %v", state.bookFluxWindow.Sum())
	}

	withMinFillToCancelRatio(0.15, func() {
		if state.bookFluxTrustworthy() {
			t.Fatal("expected untrustworthy with book flux and no trades")
		}

		state.FeedTrade(now, 5)

		if state.tradeFluxWindow.Sum() != 5 {
			t.Fatalf("expected trade flux 5, got %v", state.tradeFluxWindow.Sum())
		}

		if state.bookFluxTrustworthy() {
			t.Fatal("expected untrustworthy below fill-to-cancel ratio")
		}

		state.FeedTrade(now.Add(time.Millisecond), 45)

		if !state.bookFluxTrustworthy() {
			t.Fatalf(
				"expected trustworthy at ratio %v, book=%v trade=%v",
				state.tradeFluxWindow.Sum()/state.bookFluxWindow.Sum(),
				state.bookFluxWindow.Sum(),
				state.tradeFluxWindow.Sum(),
			)
		}
	})

	withMinFillToCancelRatio(2.5, func() {
		if state.bookFluxTrustworthy() {
			t.Fatal("expected untrustworthy when threshold exceeds ratio")
		}
	})

	equalState := NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"})
	equalState.prevBids = []market.BookLevel{{Price: 10, Volume: 50}}
	equalState.prevAsks = []market.BookLevel{{Price: 10.01, Volume: 50}}
	equalState.FeedBook(market.BookLevelsDelta{
		BidOK: true,
		AskOK: true,
		Bids:  []market.BookLevel{{Price: 10, Volume: 100}},
		Asks:  []market.BookLevel{{Price: 10.01, Volume: 50}},
	})
	equalState.FeedTrade(now, 7.5)

	withMinFillToCancelRatio(0.15, func() {
		if !equalState.bookFluxTrustworthy() {
			t.Fatal("expected trustworthy at exact fill-to-cancel ratio")
		}
	})

	state.FeedTrade(now.Add(2*time.Millisecond), 0)
	state.FeedTrade(now.Add(3*time.Millisecond), -1)

	if state.tradeFluxWindow.Sum() != 50 {
		t.Fatalf("expected zero/negative trades ignored, trade flux=%v", state.tradeFluxWindow.Sum())
	}
}

func TestFeedBookIgnoresFirstSnapshot(t *testing.T) {
	state := NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"})

	state.FeedBook(market.BookLevelsDelta{
		BidOK: true,
		AskOK: true,
		Bids:  []market.BookLevel{{Price: 10, Volume: 80}},
		Asks:  []market.BookLevel{{Price: 10.01, Volume: 20}},
	})

	if state.bookFluxWindow.Sum() != 0 {
		t.Fatalf("expected first snapshot to produce zero flux, got %v", state.bookFluxWindow.Sum())
	}
}
