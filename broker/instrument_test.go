package broker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

/*
TestInstrumentSubscribeLoadsUniverseFees verifies every online USD pair gets
its fee schedule before the complete market-data universe is subscribed.
*/
func TestInstrumentSubscribeLoadsUniverseFees(t *testing.T) {
	previousBatch := viper.Get("market.subscribe_batch")
	previousPace := viper.Get("market.subscribe_pace")
	previousQuote := viper.Get("market.quote_currency")
	previousLevel3 := viper.Get("market.l3_enabled")
	t.Cleanup(func() {
		viper.Set("market.subscribe_batch", previousBatch)
		viper.Set("market.subscribe_pace", previousPace)
		viper.Set("market.quote_currency", previousQuote)
		viper.Set("market.l3_enabled", previousLevel3)
	})

	Convey("Given two online USD instruments with complete Kraken fees", t, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("market.subscribe_batch", 200)
		viper.Set("market.subscribe_pace", "0ms")
		viper.Set("market.l3_enabled", false)
		mock := mockapi.NewMockAPI()
		So(mock.SetTradeVolumeResponse(&kraken.TradeVolume{
			Result: kraken.TradeVolumeResult{Fees: map[string]kraken.TradeVolumeFee{
				"XXBTZUSD": {Fee: decimal.NewFromFloat64(0.26)},
				"ZECUSD":   {Fee: decimal.NewFromFloat64(0.26)},
			}},
		}), ShouldBeNil)
		api := websocket.NewAPI(
			context.Background(), mock.Public(), mock.Private(), nil,
		)
		So(api.Initialize(), ShouldBeNil)
		price := broker.NewPrice(api)
		instrument := broker.NewInstrument(api, price, nil)
		instrument.On([]byte(`{
			"channel":"instrument",
			"type":"snapshot",
			"data":{
				"pairs":[
					{"symbol":"BTC/USD","quote":"USD","status":"online"},
					{"symbol":"ZEC/USD","quote":"USD","status":"online"}
				]
			}
		}`))

		Convey("Then fees load and every public subscription crosses the Conn", func() {
			So(instrument.Status(), ShouldEqual, types.READY)
			So(mock.Public().Writes(), ShouldHaveLength, 3)
			symbols, symbolsErr := mock.LastTradeVolumeSymbols()
			So(symbolsErr, ShouldBeNil)
			So(symbols, ShouldHaveLength, 2)
			So(symbols, ShouldContain, "BTC/USD")
			So(symbols, ShouldContain, "ZEC/USD")
			btcFee, feeErr := price.FeeRate("BTC/USD")
			So(feeErr, ShouldBeNil)
			So(btcFee.Fee.Float64(), ShouldEqual, 0.26)
			zecFee, feeErr := price.FeeRate("ZEC/USD")

			So(feeErr, ShouldBeNil)
			So(zecFee.Fee.Float64(), ShouldEqual, 0.26)
		})
	})
}

func TestInstrumentSubscribeConnectionFailure(t *testing.T) {
	previousBatch := viper.Get("market.subscribe_batch")
	previousPace := viper.Get("market.subscribe_pace")
	previousQuote := viper.Get("market.quote_currency")
	previousLevel3 := viper.Get("market.l3_enabled")
	t.Cleanup(func() {
		viper.Set("market.subscribe_batch", previousBatch)
		viper.Set("market.subscribe_pace", previousPace)
		viper.Set("market.quote_currency", previousQuote)
		viper.Set("market.l3_enabled", previousLevel3)
	})
	viper.Set("market.quote_currency", "USD")
	viper.Set("market.subscribe_batch", 200)
	viper.Set("market.subscribe_pace", time.Duration(0))
	viper.Set("market.l3_enabled", false)

	Convey("Given an instrument universe whose Conn rejects subscriptions", t, func() {
		mock := mockapi.NewMockAPI()
		So(mock.SetTradeVolumeResponse(&kraken.TradeVolume{
			Result: kraken.TradeVolumeResult{Fees: map[string]kraken.TradeVolumeFee{
				"XXBTZUSD": {Fee: decimal.NewFromFloat64(0.26)},
			}},
		}), ShouldBeNil)
		mock.Public().FailWrites(errors.New("subscription rejected"))
		api := websocket.NewAPI(
			context.Background(), mock.Public(), mock.Private(), nil,
		)
		So(api.Initialize(), ShouldBeNil)
		instrument := broker.NewInstrument(api, broker.NewPrice(api), nil)

		instrument.On([]byte(`{
			"channel":"instrument","type":"snapshot","data":{"pairs":[{
				"symbol":"BTC/USD","quote":"USD","status":"online"
			}]}}`))

		Convey("Then the broker refuses to report READY", func() {
			So(instrument.Status(), ShouldEqual, types.ERROR)
		})
	})
}

func TestInstrumentOn(t *testing.T) {
	Convey("Given an instrument snapshot frame", t, func() {
		mock := mockapi.NewMockAPI()
		api := websocket.NewAPI(
			context.Background(), mock.Public(), mock.Private(), nil,
		)
		So(api.Initialize(), ShouldBeNil)
		price := broker.NewPrice(api)
		instrument := broker.NewInstrument(api, price, nil)

		raw := []byte(`{"channel":"instrument","type":"snapshot","data":{"pairs":[{"symbol":"BTC/USD","base":"BTC","quote":"USD","status":"online","qty_precision":8,"qty_increment":0.00000001,"price_precision":1,"cost_precision":5,"cost_min":0.5,"tick_size":0.1,"price_increment":0.1,"qty_min":0.0001}]}}`)

		Convey("When the frame is ingested", func() {
			instrument.On(raw)

			Convey("Then the pair is cached", func() {
				pair, err := instrument.Pair("BTC/USD")
				So(err, ShouldBeNil)
				So(pair.Symbol, ShouldEqual, "BTC/USD")
				So(pair.Status, ShouldEqual, "online")
				So(pair.QtyPrecision, ShouldEqual, 8)
				So(pair.QtyIncrement, ShouldEqual, 0.00000001)
				So(pair.QtyMin, ShouldEqual, 0.0001)
				So(pair.PricePrecision, ShouldEqual, 1)
				So(pair.CostPrecision, ShouldEqual, 5)
				So(pair.CostMin.Cmp(decimal.NewFromFloat64(0.5)), ShouldEqual, 0)
				So(pair.TickSize.Cmp(decimal.NewFromFloat64(0.1)), ShouldEqual, 0)
				So(pair.PriceIncrement.Cmp(decimal.NewFromFloat64(0.1)), ShouldEqual, 0)
			})
		})
	})
}

func BenchmarkInstrumentOn(b *testing.B) {
	previousBatch := viper.Get("market.subscribe_batch")
	b.Cleanup(func() { viper.Set("market.subscribe_batch", previousBatch) })
	viper.Set("market.subscribe_batch", 0)
	mock := mockapi.NewMockAPI()
	api := websocket.NewAPI(
		context.Background(), mock.Public(), mock.Private(), nil,
	)
	_ = api.Initialize()
	price := broker.NewPrice(api)
	instrument := broker.NewInstrument(api, price, nil)
	raw := []byte(`{"channel":"instrument","type":"snapshot","data":{"pairs":[{"symbol":"BTC/USD","base":"BTC","quote":"USD","status":"online"}]}}`)

	b.ReportAllocs()

	for b.Loop() {
		instrument.On(raw)
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
		mock := mockapi.NewMockAPI()
		api := websocket.NewAPI(
			context.Background(), mock.Public(), mock.Private(), nil,
		)
		So(api.Initialize(), ShouldBeNil)
		price := broker.NewPrice(api)
		instrument := broker.NewInstrument(api, price, hub)

		instrument.On([]byte(`{
			"channel":"instrument",
			"type":"snapshot",
			"data":{"pairs":[
				{"symbol":"BTC/USD","base":"BTC","quote":"USD","status":"online"},
				{"symbol":"ETH/EUR","base":"ETH","quote":"EUR","status":"online"}
			]}
		}`))

		Convey("Then only online quote-currency pairs are published", func() {
			So(len(hub), ShouldEqual, 1)
			frame := string(<-hub)
			So(frame, ShouldContainSubstring, `"instruments"`)
			So(frame, ShouldContainSubstring, `"BTC/USD"`)
			So(frame, ShouldNotContainSubstring, `"ETH/EUR"`)
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
		mock := mockapi.NewMockAPI()
		api := websocket.NewAPI(
			context.Background(), mock.Public(), mock.Private(), nil,
		)
		So(api.Initialize(), ShouldBeNil)
		price := broker.NewPrice(api)
		instrument := broker.NewInstrument(api, price, nil)
		instrument.On([]byte(`{
			"channel":"instrument",
			"type":"snapshot",
			"data":{"pairs":[{"symbol":"BTC/USD","quote":"USD","status":"online"}]}
		}`))

		Convey("Then subscription aborts before batching", func() {
			So(instrument.Status(), ShouldEqual, types.ERROR)
		})
	})
}
