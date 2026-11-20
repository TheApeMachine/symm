package kraken

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewInstrument(t *testing.T) {
	Convey("Setup", t, func() {
		raw := []byte(`{
			"channel":"instrument",
			"type":"snapshot",
			"data":{
				"pairs":[{
					"symbol":"BTC/USD",
					"base":"BTC",
					"quote":"USD",
					"status":"online",
					"qty_precision":8,
					"qty_increment":"0.00000001",
					"price_precision":1,
					"cost_precision":5,
					"marginable":false,
					"has_index":false,
					"cost_min":"0.5",
					"tick_size":"0.1",
					"price_increment":"0.1",
					"qty_min":"0.0001"
				}]
			}
		}`)

		Convey("When NewInstrument decodes a snapshot", func() {
			instrument := NewInstrument(raw)

			Convey("pair rules decode with decimal increments", func() {
				So(instrument, ShouldNotBeNil)
				So(instrument.Channel, ShouldEqual, "instrument")
				So(len(instrument.Data.Pairs), ShouldEqual, 1)

				pair := instrument.Data.Pairs[0]
				So(pair.Symbol, ShouldEqual, "BTC/USD")
				So(pair.Base, ShouldEqual, "BTC")
				So(pair.Quote, ShouldEqual, "USD")
				So(pair.Status, ShouldEqual, "online")
				So(pair.QtyPrecision, ShouldEqual, 8)
				So(pair.QtyIncrement.String(), ShouldEqual, "0.00000001")
				So(pair.TickSize.String(), ShouldEqual, "0.1")
				So(pair.QtyMin.String(), ShouldEqual, "0.0001")
				So(pair.CostMin.String(), ShouldEqual, "0.5")
			})
		})

		Convey("When the payload is invalid JSON", func() {
			instrument := NewInstrument([]byte("not-json"))

			Convey("nil is returned", func() {
				So(instrument, ShouldBeNil)
			})
		})
	})
}

func TestNewInstrumentSubscription(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewInstrumentSubscription is marshaled", func() {
			raw, err := sonic.Marshal(NewInstrumentSubscription())

			Convey("the instrument channel is subscribed", func() {
				So(err, ShouldBeNil)
				var frame InstrumentSubscription
				So(sonic.Unmarshal(raw, &frame), ShouldBeNil)
				So(frame.Method, ShouldEqual, "subscribe")
				So(frame.Params.Channel, ShouldEqual, "instrument")
			})
		})
	})
}

func TestInstrumentPairDecimals(t *testing.T) {
	Convey("Setup", t, func() {
		inc := decimal.RequireFromString("0.01")
		pair := InstrumentPair{PriceIncrement: &inc}

		Convey("When decimal fields round-trip through sonic", func() {
			raw, err := sonic.Marshal(pair)
			So(err, ShouldBeNil)

			var decoded InstrumentPair
			So(sonic.Unmarshal(raw, &decoded), ShouldBeNil)

			Convey("PriceIncrement survives", func() {
				So(decoded.PriceIncrement.String(), ShouldEqual, "0.01")
			})
		})
	})
}

func BenchmarkNewInstrument(b *testing.B) {
	raw := []byte(`{
		"channel":"instrument",
		"type":"snapshot",
		"data":{
			"pairs":[{
				"symbol":"BTC/USD",
				"base":"BTC",
				"quote":"USD",
				"status":"online",
				"qty_precision":8,
				"qty_increment":"0.00000001",
				"price_precision":1,
				"cost_precision":5,
				"marginable":false,
				"has_index":false,
				"cost_min":"0.5",
				"tick_size":"0.1",
				"price_increment":"0.1",
				"qty_min":"0.0001"
			}]
		}
	}`)

	b.ReportAllocs()

	for b.Loop() {
		_ = NewInstrument(raw)
	}
}

func BenchmarkNewInstrumentSubscription(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_, _ = sonic.Marshal(NewInstrumentSubscription())
	}
}

func BenchmarkInstrumentPairDecimals(b *testing.B) {
	inc := decimal.RequireFromString("0.01")
	pair := InstrumentPair{PriceIncrement: &inc}

	b.ReportAllocs()

	for b.Loop() {
		raw, err := sonic.Marshal(pair)
		if err != nil {
			b.Fatal(err)
		}
		var decoded InstrumentPair
		_ = sonic.Unmarshal(raw, &decoded)
	}
}
