package trader

import (
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
)

func TestInstrumentOn(t *testing.T) {
	Convey("Given an instrument snapshot frame", t, func() {
		instrument := &Instrument{
			cache: &sync.Map{},
			quote: "EUR",
		}

		raw := []byte(`{"channel":"instrument","type":"snapshot","data":{"pairs":[{"symbol":"BTC/USD","base":"BTC","quote":"USD","status":"online"}]}}`)

		Convey("When the frame is ingested", func() {
			instrument.On(raw)

			Convey("Then the pair is cached", func() {
				pair, err := instrument.Pair("BTC/USD")
				So(err, ShouldBeNil)
				So(pair.Symbol, ShouldEqual, "BTC/USD")
				So(pair.Status, ShouldEqual, "online")
			})
		})
	})
}

func BenchmarkInstrumentOn(b *testing.B) {
	instrument := &Instrument{
		cache: &sync.Map{},
		quote: "EUR",
	}
	raw := []byte(`{"channel":"instrument","type":"snapshot","data":{"pairs":[{"symbol":"BTC/USD","base":"BTC","quote":"USD","status":"online"}]}}`)

	b.ReportAllocs()

	for b.Loop() {
		instrument.On(raw)
	}
}

func TestInstrumentSubscribeRejectsInvalidBatchSize(t *testing.T) {
	Convey("Given an invalid subscribe batch size", t, func() {
		viper.Set("market.subscribe_batch", 0)
		instrument := &Instrument{
			cache: &sync.Map{},
			quote: "USD",
		}
		instrument.cache.Store("BTC/USD", kraken.InstrumentPair{
			Symbol: "BTC/USD",
			Quote:  "USD",
			Status: "online",
		})

		err := instrument.Subscribe()

		Convey("Then subscription aborts before batching", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestInstrumentTier(t *testing.T) {
	Convey("Given fewer observed tickers than the configured heavy tier", t, func() {
		viper.Set("market.universe.trading_tier_size", 2)
		price := broker.NewPrice(nil)
		price.TickerAck([]byte(`{
			"channel":"ticker",
			"type":"snapshot",
			"data":[
				{"symbol":"DEEP/USD","bid":"10","bid_qty":20,"ask":"11","ask_qty":20,"last":"10.5","volume":100,"vwap":10.5,"timestamp":"2026-07-14T09:00:00Z"}
			]
		}`))
		instrument := &Instrument{
			price:   price,
			symbols: []string{"DEEP/USD", "PENDING/USD"},
		}

		symbols, ready, err := instrument.Tier(nil)

		Convey("Then activation waits without reporting normal startup as an error", func() {
			So(err, ShouldBeNil)
			So(ready, ShouldBeFalse)
			So(symbols, ShouldBeNil)
		})
	})

	Convey("Given a complete ticker snapshot wider than the heavy tier", t, func() {
		viper.Set("market.universe.trading_tier_size", 2)
		price := broker.NewPrice(nil)
		price.TickerAck([]byte(`{
			"channel":"ticker",
			"type":"snapshot",
			"data":[
				{"symbol":"THIN/USD","bid":"10","bid_qty":1,"ask":"11","ask_qty":1,"last":"10.5","volume":100,"vwap":10.5,"timestamp":"2026-07-14T09:00:00Z"},
				{"symbol":"DEEP/USD","bid":"10","bid_qty":20,"ask":"11","ask_qty":20,"last":"10.5","volume":100,"vwap":10.5,"timestamp":"2026-07-14T09:00:00Z"},
				{"symbol":"MID/USD","bid":"10","bid_qty":10,"ask":"11","ask_qty":10,"last":"10.5","volume":100,"vwap":10.5,"timestamp":"2026-07-14T09:00:00Z"}
			]
		}`))
		instrument := &Instrument{
			price:   price,
			symbols: []string{"THIN/USD", "DEEP/USD", "MID/USD"},
		}

		symbols, ready, err := instrument.Tier(nil)

		Convey("Then executable depth selects a deterministic bounded tier", func() {
			So(err, ShouldBeNil)
			So(ready, ShouldBeTrue)
			So(symbols, ShouldResemble, []string{"DEEP/USD", "MID/USD"})
		})
	})

	Convey("Given an open holding outside the depth-ranked tier", t, func() {
		viper.Set("market.universe.trading_tier_size", 2)
		price := broker.NewPrice(nil)
		price.TickerAck([]byte(`{
			"channel":"ticker",
			"type":"snapshot",
			"data":[
				{"symbol":"HELD/USD","bid":"10","bid_qty":1,"ask":"11","ask_qty":1,"last":"10.5","volume":100,"vwap":10.5,"timestamp":"2026-07-14T09:00:00Z"},
				{"symbol":"DEEP/USD","bid":"10","bid_qty":20,"ask":"11","ask_qty":20,"last":"10.5","volume":100,"vwap":10.5,"timestamp":"2026-07-14T09:00:00Z"},
				{"symbol":"MID/USD","bid":"10","bid_qty":10,"ask":"11","ask_qty":10,"last":"10.5","volume":100,"vwap":10.5,"timestamp":"2026-07-14T09:00:00Z"}
			]
		}`))
		instrument := &Instrument{
			price:   price,
			symbols: []string{"HELD/USD", "DEEP/USD", "MID/USD"},
		}

		symbols, ready, err := instrument.Tier([]string{"HELD/USD"})

		Convey("Then the holding is retained before depth fills remaining capacity", func() {
			So(err, ShouldBeNil)
			So(ready, ShouldBeTrue)
			So(symbols, ShouldResemble, []string{"HELD/USD", "DEEP/USD"})
		})
	})
}

func BenchmarkInstrumentTier(b *testing.B) {
	viper.Set("market.universe.trading_tier_size", 2)
	price := broker.NewPrice(nil)
	price.TickerAck([]byte(`{
		"channel":"ticker",
		"type":"snapshot",
		"data":[
			{"symbol":"THIN/USD","bid":"10","bid_qty":1,"ask":"11","ask_qty":1,"last":"10.5","volume":100,"vwap":10.5,"timestamp":"2026-07-14T09:00:00Z"},
			{"symbol":"DEEP/USD","bid":"10","bid_qty":20,"ask":"11","ask_qty":20,"last":"10.5","volume":100,"vwap":10.5,"timestamp":"2026-07-14T09:00:00Z"},
			{"symbol":"MID/USD","bid":"10","bid_qty":10,"ask":"11","ask_qty":10,"last":"10.5","volume":100,"vwap":10.5,"timestamp":"2026-07-14T09:00:00Z"}
		]
	}`))
	instrument := &Instrument{
		price:   price,
		symbols: []string{"THIN/USD", "DEEP/USD", "MID/USD"},
	}

	b.ReportAllocs()

	for b.Loop() {
		symbols, ready, err := instrument.Tier(nil)

		if err != nil || !ready || len(symbols) != 2 {
			b.Fatal("incomplete trading tier")
		}
	}
}
