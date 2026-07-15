package trader

import (
	"context"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
)

/*
TestInstrumentSubscribeLoadsUniverseFees verifies every online USD pair gets
its fee schedule before the complete market-data universe is subscribed.
*/
func TestInstrumentSubscribeLoadsUniverseFees(t *testing.T) {
	Convey("Given two online USD instruments with complete Kraken fees", t, func() {
		viper.Set("market.subscribe_batch", 200)
		mock := tests.NewMockAPI()
		So(mock.SetTradeVolumeResponse(&kraken.TradeVolume{
			Result: kraken.TradeVolumeResult{Fees: map[string]kraken.TradeVolumeFees{
				"XXBTZUSD": {Fee: "0.2600"},
				"ZECUSD":   {Fee: "0.2600"},
			}},
		}), ShouldBeNil)
		api := websocket.NewAPI(
			context.Background(), mock.Public(), mock.Private(), nil,
		)
		So(api.Initialize(), ShouldBeNil)
		price := broker.NewPrice(api)
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
			btcFee, feeErr := price.FeeFraction("BTC/USD")
			So(feeErr, ShouldBeNil)
			So(btcFee.Float64(), ShouldEqual, 0.0026)
			zecFee, feeErr := price.FeeFraction("ZEC/USD")

			So(feeErr, ShouldBeNil)
			So(zecFee.Float64(), ShouldEqual, 0.0026)
		})
	})
}

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
