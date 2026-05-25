package ui

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/market"
)

func TestCandleQueuePublishTickerUpdate(t *testing.T) {
	Convey("Given a candle mirror wired to ui", t, func() {
		ctx, cancel := contextWithCancel(t)
		defer cancel()

		uiGroup := mustBroadcastGroup(t, ctx, t.Name()+"-ui")
		defer uiGroup.Close()

		capture := uiGroup.Subscribe("capture", 8)
		chartWatch := NewChartWatch("BTC/EUR")
		candleQueue, err := NewCandleQueue(
			uiGroup,
			chartWatch,
			5*time.Second,
		)
		So(err, ShouldBeNil)

		err = candleQueue.PublishTickerUpdate(
			"BTC/EUR",
			100,
			"2026-05-25T12:00:01.000000000Z",
		)
		So(err, ShouldBeNil)

		payload := waitPayload(t, capture)

		Convey("It should publish ticker-driven candle bars through ui", func() {
			So(payload["event"], ShouldEqual, "candle_bar")
			So(payload["symbol"], ShouldEqual, "BTC/EUR")
			So(payload["close"], ShouldEqual, 100)
		})
	})
}

func TestCandleQueueTradeBroadcast(t *testing.T) {
	Convey("Given a candle mirror wired to ui", t, func() {
		ctx, cancel := contextWithCancel(t)
		defer cancel()

		uiGroup := mustBroadcastGroup(t, ctx, t.Name()+"-ui")
		defer uiGroup.Close()

		capture := uiGroup.Subscribe("capture", 8)
		chartWatch := NewChartWatch("BTC/EUR")
		candleQueue, err := NewCandleQueue(
			uiGroup,
			chartWatch,
			5*time.Second,
		)
		So(err, ShouldBeNil)

		err = candleQueue.PublishTradeUpdate(engine.TradeUpdate{
			Symbol:    "BTC/EUR",
			UpdatedAt: time.Unix(1_700_000_002, 0),
			Ticks: []market.TradeTick{
				{
					Symbol:    "BTC/EUR",
					Price:     100,
					Volume:    1,
					Side:      "buy",
					Timestamp: time.Unix(1_700_000_002, 0),
				},
			},
		})
		So(err, ShouldBeNil)

		payload := waitPayload(t, capture)

		Convey("It should publish candle bars through the ui group", func() {
			So(payload["event"], ShouldEqual, "candle_bar")
			So(payload["symbol"], ShouldEqual, "BTC/EUR")
			So(payload["sec"], ShouldEqual, int64(1_700_000_000))
			So(payload["close"], ShouldEqual, 100)
			So(payload["volume"], ShouldEqual, 1)
		})
	})
}

func TestCandleQueueIgnoresUnwatchedSymbols(t *testing.T) {
	Convey("Given an unwatched trade update", t, func() {
		ctx, cancel := contextWithCancel(t)
		defer cancel()

		uiGroup := mustBroadcastGroup(t, ctx, t.Name()+"-ui")
		defer uiGroup.Close()

		candleStream, err := NewCandleStream(5 * time.Second)
		So(err, ShouldBeNil)

		candleQueue := &CandleQueue{
			stream: candleStream,
			watch:  NewChartWatch("BTC/EUR"),
			ui:     uiGroup,
		}

		err = candleQueue.handleTradeTick("ETH/EUR", market.TradeTick{
			Symbol:    "ETH/EUR",
			Price:     100,
			Volume:    1,
			Side:      "buy",
			Timestamp: time.Unix(1_700_000_002, 0),
		})

		Convey("It should not create an unwatched candle", func() {
			So(err, ShouldBeNil)
			So(candleStream.bySymbol, ShouldHaveLength, 0)
		})
	})
}

func BenchmarkCandleQueueHandleTradeTick(benchmark *testing.B) {
	ctx, cancel := contextWithCancel(benchmark)
	defer cancel()

	uiGroup := mustBroadcastGroup(benchmark, ctx, benchmark.Name()+"-ui")
	defer uiGroup.Close()

	candleStream, err := NewCandleStream(5 * time.Second)

	if err != nil {
		benchmark.Fatal(err)
	}

	candleQueue := &CandleQueue{
		stream: candleStream,
		watch:  NewChartWatch("BTC/EUR"),
		ui:     uiGroup,
	}
	tick := market.TradeTick{
		Symbol:    "BTC/EUR",
		Price:     100,
		Volume:    1,
		Side:      "buy",
		Timestamp: time.Unix(1_700_000_002, 0),
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		if err := candleQueue.handleTradeTick("BTC/EUR", tick); err != nil {
			benchmark.Fatal(err)
		}
	}
}

type testingFataler interface {
	Fatal(args ...any)
}

func contextWithCancel(test testingFataler) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	return ctx, cancel
}

func mustBroadcastGroup(
	test testingFataler,
	ctx context.Context,
	id string,
) *qpool.BroadcastGroup {
	group, err := qpool.NewBroadcastGroup(ctx, id, time.Minute)

	if err != nil {
		test.Fatal(err)
	}

	return group
}

func waitPayload(
	test testingFataler,
	subscriber *qpool.Subscriber,
) map[string]any {
	select {
	case value := <-subscriber.Incoming:
		payload, ok := value.Value.(map[string]any)

		if !ok {
			test.Fatal("expected map payload")
		}

		return payload
	case <-time.After(time.Second):
		test.Fatal("timed out waiting for ui candle")
	}

	return nil
}
