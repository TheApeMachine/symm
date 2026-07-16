package broker

import (
	"context"
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
TestInstrumentSubscribeLoadsUniverseFees verifies every online USD pair gets
its fee schedule before the complete market-data universe is subscribed.
*/
func TestInstrumentSubscribeLoadsUniverseFees(t *testing.T) {
	previousBatch := viper.Get("market.subscribe_batch")
	t.Cleanup(func() { viper.Set("market.subscribe_batch", previousBatch) })

	Convey("Given two online USD instruments with complete Kraken fees", t, func() {
		viper.Set("market.subscribe_batch", 200)
		mock := tests.NewMockAPI()
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
		price := NewPrice(api)
		instrument := NewInstrument(api, price, nil)
		instrument.quote = "USD"
		instrument.cache.Store("BTC/USD", kraken.InstrumentPair{
			Symbol: "BTC/USD", Quote: "USD", Status: "online",
		})
		instrument.cache.Store("ZEC/USD", kraken.InstrumentPair{
			Symbol: "ZEC/USD", Quote: "USD", Status: "online",
		})

		err := instrument.Subscribe()

		Convey("Then all eligible symbols have fees before subscription transport is attempted", func() {
			So(err, ShouldNotBeNil)
			So(mock.LastTradeVolumeSymbols(), ShouldResemble, []string{"BTC/USD", "ZEC/USD"})
			btcFee, feeErr := price.FeeRate("BTC/USD")
			So(feeErr, ShouldBeNil)
			So(btcFee.Fee.Float64(), ShouldEqual, 0.26)
			zecFee, feeErr := price.FeeRate("ZEC/USD")

			So(feeErr, ShouldBeNil)
			So(zecFee.Fee.Float64(), ShouldEqual, 0.26)
		})
	})
}

func TestInstrumentOn(t *testing.T) {
	Convey("Given an instrument snapshot frame", t, func() {
		instrument := &Instrument{
			cache: &sync.Map{},
			quote: "EUR",
		}

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
	instrument := &Instrument{
		cache: &sync.Map{},
		quote: "EUR",
	}
	instrument.status.Store(types.READY)
	raw := []byte(`{"channel":"instrument","type":"snapshot","data":{"pairs":[{"symbol":"BTC/USD","base":"BTC","quote":"USD","status":"online"}]}}`)

	b.ReportAllocs()

	for b.Loop() {
		instrument.On(raw)
	}
}

func TestInstrumentSubscribeRejectsInvalidBatchSize(t *testing.T) {
	previousBatch := viper.Get("market.subscribe_batch")
	t.Cleanup(func() { viper.Set("market.subscribe_batch", previousBatch) })

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
