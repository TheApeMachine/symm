package tests

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/callback"
	sdkkraken "github.com/theapemachine/api-go/v2/pkg/kraken"
	"github.com/theapemachine/symm/kraken"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestMarketPace(t *testing.T) {
	Convey("Given an undriven fixture", t, func() {
		market := NewMarket(t.Context(), []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100, 42),
		})
		defer market.Close()
		market.pace(time.Now().Add(time.Hour))

		Convey("Direct fixture use should not wait for wall-clock pacing", func() {
			So(market.clockSet, ShouldBeFalse)
		})
	})
}

func TestMarketPublishSample(t *testing.T) {
	Convey("Given two replay observations in one candle interval", t, func() {
		symbol := testtypes.NewSymbol("SIM1/USD", 100, 93)
		market := NewMarket(context.Background(), []*testtypes.Symbol{symbol})
		defer market.Close()
		candles := make(chan *kraken.OHLC, 2)
		candleHandler := market.Public.Client().OnReceived.Recurring(
			func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
				candle := kraken.NewOHLC(event.Data.Bytes())

				if candle.Channel == "ohlc" {
					candles <- candle
				}
			},
		)
		defer market.Public.Client().OnReceived.Deregister(candleHandler)
		begin := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
		first := testtypes.Sample{
			Symbol: symbol.Pair, AggressorSide: "buy",
			Bid: 99, BidQty: 10, Ask: 101, AskQty: 12, Last: 100,
			Volume: 2, StepVolume: 2, VWAP: 100, Low: 100, High: 100,
			Timestamp: begin.Add(10 * time.Second),
		}
		second := testtypes.Sample{
			Symbol: symbol.Pair, AggressorSide: "buy",
			Bid: 100, BidQty: 11, Ask: 102, AskQty: 13, Last: 101,
			Volume: 5, StepVolume: 3, VWAP: 100.6, Low: 100, High: 101,
			Change: 1, ChangePct: 1, Timestamp: begin.Add(20 * time.Second),
		}

		So(market.PublishSample(first), ShouldBeNil)
		So(market.PublishSample(second), ShouldBeNil)
		<-candles
		candle := <-candles

		Convey("All channels should reflect the same replayed trades", func() {
			So(candle.Data[0].Open, ShouldEqual, 100.0)
			So(candle.Data[0].High, ShouldEqual, 101.0)
			So(candle.Data[0].Low, ShouldEqual, 100.0)
			So(candle.Data[0].Close, ShouldEqual, 101.0)
			So(candle.Data[0].Trades, ShouldEqual, int64(2))
			So(candle.Data[0].Volume, ShouldEqual, 5.0)
			So(candle.Data[0].Vwap, ShouldEqual, 100.6)
			So(market.tick, ShouldEqual, uint64(2))
		})

		Convey("Stale or incoherent replay data should be rejected", func() {
			So(market.PublishSample(second), ShouldNotBeNil)
			invalid := second
			invalid.Timestamp = invalid.Timestamp.Add(time.Second)
			invalid.Bids = []testtypes.DepthLevel{{
				Price: invalid.Bid + 1, Quantity: invalid.BidQty,
			}}
			So(market.PublishSample(invalid), ShouldNotBeNil)
		})
	})
}

func TestMarketPublish(t *testing.T) {
	Convey("Given two markets with the same complete replay identity", t, func() {
		first := NewMarket(t.Context(), []*testtypes.Symbol{
			testtypes.NewSymbol("SIM/USD", 100, 17),
		})
		second := NewMarket(t.Context(), []*testtypes.Symbol{
			testtypes.NewSymbol("SIM/USD", 100, 17),
		})
		defer first.Close()
		defer second.Close()
		first.Tick()
		second.Tick()
		firstSample, _ := first.LastSample("SIM/USD")
		secondSample, _ := second.LastSample("SIM/USD")

		Convey("Prices and timestamps should be identical", func() {
			So(firstSample, ShouldResemble, secondSample)
			So(first.Report().PublicTransport.Frames,
				ShouldResemble, second.Report().PublicTransport.Frames)
			So(firstSample.Timestamp,
				ShouldEqual, testtypes.DefaultScenarioStart.Add(100*time.Millisecond))
		})
	})
}
