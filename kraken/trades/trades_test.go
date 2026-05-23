package trades

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
)

func TestTradesBuyPressure(t *testing.T) {
	convey.Convey("Given a batch of trade ticks", t, func() {
		observer := &Trades{
			symbols: make(map[string]tradeState),
		}
		batch := []market.TradeTick{
			{Symbol: "PUMP/EUR", Side: "buy", Volume: 3, Timestamp: time.Now()},
			{Symbol: "PUMP/EUR", Side: "sell", Volume: 1, Timestamp: time.Now()},
		}

		convey.Convey("It should compute normalized buy pressure", func() {
			convey.So(observer.applyTicks(batch), convey.ShouldBeNil)

			pressure, ok := observer.BuyPressure("PUMP/EUR")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(pressure, convey.ShouldEqual, 0.5)
		})
	})
}

func TestTradesBatchVolume(t *testing.T) {
	convey.Convey("Given a batch of trade ticks", t, func() {
		observer := &Trades{
			symbols: make(map[string]tradeState),
		}
		batch := []market.TradeTick{
			{Symbol: "PUMP/EUR", Side: "buy", Volume: 3, Timestamp: time.Now()},
			{Symbol: "PUMP/EUR", Side: "sell", Volume: 1, Timestamp: time.Now()},
		}

		convey.Convey("It should sum executed volume", func() {
			convey.So(observer.applyTicks(batch), convey.ShouldBeNil)

			volume, ok := observer.BatchVolume("PUMP/EUR")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(volume, convey.ShouldEqual, 4)
		})
	})
}

func TestTradesApplyTicks(t *testing.T) {
	convey.Convey("Given trade ticks for one symbol", t, func() {
		observer := &Trades{
			symbols: make(map[string]tradeState),
		}
		ticks := []market.TradeTick{
			{Symbol: "PUMP/EUR", Side: "buy", Volume: 3, Timestamp: time.Now()},
			{Symbol: "PUMP/EUR", Side: "sell", Volume: 1, Timestamp: time.Now()},
		}

		convey.Convey("It should store buy pressure and batch volume", func() {
			convey.So(observer.applyTicks(ticks), convey.ShouldBeNil)

			pressure, ok := observer.BuyPressure("PUMP/EUR")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(pressure, convey.ShouldEqual, 0.5)

			volume, ok := observer.BatchVolume("PUMP/EUR")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(volume, convey.ShouldEqual, 4)
		})
	})
}

func TestTradesHandleFrame(t *testing.T) {
	convey.Convey("Given a Kraken trade websocket frame", t, func() {
		observer := &Trades{
			symbols: make(map[string]tradeState),
		}

		convey.Convey("It should apply per-symbol pressure", func() {
			convey.So(observer.handleFrame(nil, []byte(`{
				"channel":"trade",
				"type":"update",
				"data":[
					{"symbol":"BTC/EUR","side":"buy","qty":0.25,"price":50000.1,"timestamp":"2026-05-23T02:00:00.123456789Z"},
					{"symbol":"BTC/EUR","side":"sell","qty":0.10,"price":50000.2,"timestamp":"2026-05-23T02:00:00.223456789Z"}
				]
			}`)), convey.ShouldBeNil)

			pressure, ok := observer.BuyPressure("BTC/EUR")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(pressure, convey.ShouldAlmostEqual, 0.428571, 0.0001)
		})
	})
}

func TestTradesRecentTicks(t *testing.T) {
	convey.Convey("Given stored trade ticks", t, func() {
		start := time.Unix(0, 0)
		observer := &Trades{
			symbols: make(map[string]tradeState),
		}
		batch := []market.TradeTick{
			{Symbol: "PUMP/EUR", Side: "buy", Volume: 1, Timestamp: start},
			{Symbol: "PUMP/EUR", Side: "buy", Volume: 1, Timestamp: start.Add(time.Second)},
		}

		convey.Convey("It should return timestamped events", func() {
			convey.So(observer.applyTicks(batch), convey.ShouldBeNil)

			ticks, ok := observer.RecentTicks("PUMP/EUR", time.Time{})
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(len(ticks), convey.ShouldEqual, 2)

			filtered, ok := observer.RecentTicks("PUMP/EUR", start.Add(500*time.Millisecond))
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(len(filtered), convey.ShouldEqual, 1)
		})
	})
}

func BenchmarkTradesBuyPressure(b *testing.B) {
	observer := &Trades{
		symbols: make(map[string]tradeState),
	}
	batch := []market.TradeTick{
		{Symbol: "BTC/EUR", Side: "buy", Volume: 3, Timestamp: time.Now()},
		{Symbol: "BTC/EUR", Side: "sell", Volume: 1, Timestamp: time.Now()},
	}

	if err := observer.applyTicks(batch); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, ok := observer.BuyPressure(batch[0].Symbol); !ok {
			b.Fatal("buy pressure not found")
		}
	}
}
