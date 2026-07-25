package broker_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

/*
TestInstrumentSubscribeLoadsUniverseFees verifies every online USD pair gets
its fee schedule before the complete market-data universe is subscribed.
*/
func TestInstrumentSubscribeLoadsUniverseFees(t *testing.T) {
	Convey("Given two simulated instruments with complete Kraken fees", t, func() {
		market := tests.NewMarket(t.Context(), 2)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})

		Convey("Then fees load and every public subscription crosses the Conn", func() {
			So(wired.Instrument.Status(), ShouldEqual, types.READY)
			So(market.Public.Subscriptions("trade"), ShouldResemble, market.Symbols)
			So(market.Public.Subscriptions("book"), ShouldResemble, market.Symbols)
			So(market.Public.Subscriptions("ticker"), ShouldResemble, market.Symbols)
			posts := market.Private.Posts()
			So(posts, ShouldHaveLength, 1)
			var request kraken.TradeVolumeRequest
			So(sonic.Unmarshal(posts[0], &request), ShouldBeNil)

			for _, symbol := range market.Symbols {
				So(request.Pair, ShouldContainSubstring, symbol)
				fee, err := wired.Price.FeeRate(symbol)
				So(err, ShouldBeNil)
				So(fee.Fee.Float64(), ShouldEqual, 0.26)
			}
		})
	})
}

/*
TestInstrumentSubscribeConnectionFailure proves a Conn rejection prevents the
production graph from reporting a usable instrument registry.
*/
func TestInstrumentSubscribeConnectionFailure(t *testing.T) {
	Convey("Given a simulated venue whose Conn rejects subscriptions", t, func() {
		market := tests.NewMarket(t.Context(), 1)
		market.Public.FailWrites(errors.New("subscription rejected"))
		Reset(market.Close)
		wired, err := stack.NewBooter(t.Context()).Test(market)

		Convey("Then the production graph refuses to boot READY", func() {
			So(wired, ShouldBeNil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "subscription rejected")
		})
	})
}

/*
TestInstrumentOn proves the production subscription graph preserves every
simulated Kraken pair's exact execution rules and requested market channels.
*/
func TestInstrumentOn(t *testing.T) {
	Convey("Given production instrument bootstrap from a simulated market", t, func() {
		market := tests.NewMarket(t.Context(), 2)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})
		So(market.Public.Subscribed("instrument"), ShouldBeTrue)
		So(market.Public.Subscriptions("trade"), ShouldResemble, market.Symbols)
		So(market.Public.Subscriptions("book"), ShouldResemble, market.Symbols)
		So(market.Public.Subscriptions("ticker"), ShouldResemble, market.Symbols)
		So(market.Level3.Subscriptions("level3"), ShouldResemble, market.Symbols)
		So(market.Paper.Subscribed("balances"), ShouldBeTrue)
		So(market.Paper.Subscribed("executions"), ShouldBeTrue)

		Convey("Every requested pair should retain Kraken execution rules", func() {
			So(wired.Instrument.Pairs(), ShouldHaveLength, len(market.Symbols))

			for _, symbol := range market.Symbols {
				pair, err := wired.Instrument.Pair(symbol)
				So(err, ShouldBeNil)
				So(pair.Symbol, ShouldEqual, symbol)
				So(pair.Status, ShouldEqual, "online")
				So(pair.QtyPrecision, ShouldEqual, 8)
				So(pair.QtyIncrement.String(), ShouldEqual, "0.00000001")
				So(pair.QtyMin.String(), ShouldEqual, "0.0001")
				So(pair.PricePrecision, ShouldEqual, 2)
				So(pair.CostPrecision, ShouldEqual, 5)
				So(pair.CostMin.Cmp(decimal.NewFromFloat64(0.5)), ShouldEqual, 0)
				So(pair.TickSize.Float64(), ShouldEqual, 0.01)
				So(pair.PriceIncrement.Float64(), ShouldEqual, 0.01)
			}
		})
	})
}

/*
BenchmarkInstrumentOn measures instrument snapshot ingestion without boot or
transport scheduling cost.
*/
func BenchmarkInstrumentOn(b *testing.B) {
	previousBatch := viper.Get("market.subscribe_batch")
	b.Cleanup(func() { viper.Set("market.subscribe_batch", previousBatch) })
	viper.Set("market.subscribe_batch", 0)
	public := mockapi.NewConn(b.Context())
	private := mockapi.NewConn(b.Context())
	api := websocket.NewAPI(
		context.Background(), public, private, nil,
	)
	if err := api.Initialize(); err != nil {
		b.Fatal(err)
	}
	price := broker.NewPrice(api)
	instrument := broker.NewInstrument(api, price, nil, config.Fixture().Market)
	raw := []byte(`{"channel":"instrument","type":"snapshot","data":{"pairs":[{"symbol":"BTC/USD","base":"BTC","quote":"USD","status":"online"}]}}`)

	b.ReportAllocs()

	for b.Loop() {
		instrument.On(kraken.NewInstrument(raw))
	}
}

/*
TestInstrumentPublishEmitsUniverse verifies the terminal receives searchable
online quote pairs after an instrument snapshot is ingested.
*/
func TestInstrumentPublishEmitsUniverse(t *testing.T) {
	previousQuote := viper.Get("market.quote_currency")
	previousBatch := viper.Get("market.subscribe_batch")
	t.Cleanup(func() {
		viper.Set("market.quote_currency", previousQuote)
		viper.Set("market.subscribe_batch", previousBatch)
	})

	Convey("Given an instrument registry with a UI hub", t, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("market.subscribe_batch", 0)
		hub := make(chan []byte, 1)
		public := mockapi.NewConn(t.Context())
		private := mockapi.NewConn(t.Context())
		api := websocket.NewAPI(
			context.Background(), public, private, nil,
		)
		So(api.Initialize(), ShouldBeNil)
		price := broker.NewPrice(api)
		instrument := broker.NewInstrument(api, price, hub, config.Fixture().Market)

		instrument.On(kraken.NewInstrument([]byte(`{
			"channel":"instrument",
			"type":"snapshot",
			"data":{"pairs":[
				{"symbol":"BTC/USD","base":"BTC","quote":"USD","status":"online"},
				{"symbol":"ETH/EUR","base":"ETH","quote":"EUR","status":"online"}
			]}
		}`)))

		Convey("Then only online quote-currency pairs are published", func() {
			So(len(hub), ShouldEqual, 1)
			frame := string(<-hub)
			So(frame, ShouldContainSubstring, `"instruments"`)
			So(frame, ShouldContainSubstring, `"BTC/USD"`)
			So(frame, ShouldNotContainSubstring, `"ETH/EUR"`)
		})
	})
}

/*
TestInstrumentSubscribeRejectsInvalidBatchSize proves invalid batching cannot
leave an Instrument reporting READY after it receives a universe.
*/
func TestInstrumentPair(t *testing.T) {
	Convey("Given a remembered instrument pair", t, func() {
		instrument := broker.NewInstrument(
			nil, nil, nil, config.Fixture().Market,
		)
		qty := decimal.NewFromFloat64(0.0001)
		instrument.Remember(kraken.InstrumentPair{
			Symbol:       "ETH/USD",
			Base:         "ETH",
			Quote:        "USD",
			QtyIncrement: qty,
		})

		Convey("Pair returns an independent decimal copy", func() {
			remembered := qty
			pair, err := instrument.Pair("ETH/USD")
			So(err, ShouldBeNil)
			So(pair.QtyIncrement, ShouldNotPointTo, remembered)
			pair.QtyIncrement = decimal.NewFromInt64(9)
			So(pair.QtyIncrement.Float64(), ShouldEqual, 9)
			again, err := instrument.Pair("ETH/USD")
			So(err, ShouldBeNil)
			So(again.QtyIncrement, ShouldNotPointTo, remembered)
			So(again.QtyIncrement, ShouldNotPointTo, pair.QtyIncrement)
			So(again.QtyIncrement.Float64(), ShouldEqual, 0.0001)
		})
	})
}

func TestInstrumentSubscribeRejectsInvalidBatchSize(t *testing.T) {
	previousBatch := viper.Get("market.subscribe_batch")
	previousQuote := viper.Get("market.quote_currency")
	t.Cleanup(func() {
		viper.Set("market.subscribe_batch", previousBatch)
		viper.Set("market.quote_currency", previousQuote)
	})

	Convey("Given an invalid subscribe batch size", t, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("market.subscribe_batch", 0)
		public := mockapi.NewConn(t.Context())
		private := mockapi.NewConn(t.Context())
		api := websocket.NewAPI(
			context.Background(), public, private, nil,
		)
		So(api.Initialize(), ShouldBeNil)
		price := broker.NewPrice(api)
		instrument := broker.NewInstrument(api, price, nil, config.Fixture().Market)
		instrument.On(kraken.NewInstrument([]byte(`{
			"channel":"instrument",
			"type":"snapshot",
			"data":{"pairs":[{"symbol":"BTC/USD","quote":"USD","status":"online"}]}
		}`)))

		Convey("Then subscription aborts before batching", func() {
			So(instrument.Status(), ShouldEqual, types.ERROR)
		})
	})
}
