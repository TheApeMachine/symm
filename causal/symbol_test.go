package causal

import (
	"sync"
	"testing"
	"time"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trade"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCausalSymbolBuildSampleVelocity(t *testing.T) {
	Convey("Given two committed samples with a price move", t, func() {
		state := NewCausalSymbol(asset.Pair{Wsname: "BTC/EUR"}, engine.DefaultCalibrationParams())
		start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		first, _ := state.buildSample(0.1, 1, 0.5, 100, start)
		state.commitSample(first, 100, start)

		second, _ := state.buildSample(
			0.1, 1, 0.5, 110, start.Add(10*time.Second),
		)

		Convey("It should use the prior committed price for velocity", func() {
			So(first.value(priceVelocityNode), ShouldEqual, 0)
			So(second.value(priceVelocityNode), ShouldAlmostEqual, 0.01, 1e-9)
		})
	})
}

func TestCausalSymbolConcurrentFeedAndMeasure(t *testing.T) {
	state := NewCausalSymbol(asset.Pair{Wsname: "BTC/EUR"}, engine.DefaultCalibrationParams())
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var waiters sync.WaitGroup

	waiters.Go(func() {
		for index := range 128 {
			state.FeedTicker(market.TickerRow{
				Last:      100 + float64(index)*0.01,
				Bid:       99.9,
				Ask:       100.1,
				Volume:    10,
				ChangePct: 0.1,
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
				Bids: []market.BookLevel{{Price: 99.9, Volume: 2}},
				Asks: []market.BookLevel{{Price: 100.1, Volume: 1}},
			})
		}
	})
	waiters.Go(func() {
		for index := range 128 {
			state.Measure(0.2, now.Add(time.Duration(index)*time.Millisecond))
		}
	})

	waiters.Wait()
}
