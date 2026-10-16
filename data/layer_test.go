package data

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type sampleTrade struct {
	Symbol string  `json:"symbol"`
	Price  float64 `json:"price"`
	Qty    float64 `json:"qty"`
}

func TestLayer(t *testing.T) {
	Convey("Given a Value with diverse layers", t, func() {
		val := NewValue("kraken", "trade", "BTC/USD")

		layer0 := []byte(`{"symbol":"BTC/USD","price":67500.5,"qty":1.25}`)
		layer1 := []byte(`[-0.045,0.123,0.890]`)
		val.Append(layer0, layer1)

		Convey("Layer unmarshals into a struct", func() {
			trade := Layer[sampleTrade](val, 0)
			So(trade, ShouldNotBeNil)
			So(trade.Symbol, ShouldEqual, "BTC/USD")
			So(trade.Price, ShouldEqual, 67500.5)
		})

		Convey("Layer unmarshals into a slice of floats", func() {
			snm := Layer[[]float64](val, 1)
			So(snm, ShouldNotBeNil)
			So(len(*snm), ShouldEqual, 3)
			So((*snm)[0], ShouldEqual, -0.045)
		})

		Convey("Layer retrieves raw bytes with zero unmarshaling", func() {
			raw := Layer[[]byte](val, 0)
			So(raw, ShouldNotBeNil)
			So(string(*raw), ShouldEqual, string(layer0))
		})
	})
}

func TestAttribute(t *testing.T) {
	Convey("Given a mixed JSON payload", t, func() {
		payload := []byte(`{
			"channel": "trade",
			"data": [
				{"symbol": "BTC/USD", "price": "67500.50", "qty": 1.25, "active": true, "count": 42}
			]
		}`)

		Convey("Attribute extracts native numbers", func() {
			qty := Attribute[float64](payload, "data", 0, "qty")
			So(qty, ShouldNotBeNil)
			So(qty, ShouldEqual, 1.25)
		})

		Convey("Attribute automatically parses string-encoded numbers", func() {
			priceStr := Attribute[string](payload, "data", 0, "price")
			So(priceStr, ShouldEqual, "67500.50")

			priceFloat := Attribute[float64](payload, "data", 0, "price")
			So(priceFloat, ShouldEqual, 67500.50)
		})

		Convey("Attribute extracts strings, booleans, and integers", func() {
			sym := Attribute[string](payload, "data", 0, "symbol")
			So(sym, ShouldEqual, "BTC/USD")

			active := Attribute[bool](payload, "data", 0, "active")
			So(active, ShouldBeTrue)

			count := Attribute[int](payload, "data", 0, "count")
			So(count, ShouldEqual, 42)

			count64 := Attribute[int64](payload, "data", 0, "count")
			So(count64, ShouldEqual, int64(42))
		})

		Convey("Attribute safely handles missing keys and AttributeOk", func() {
			missingStr := Attribute[string](payload, "nonexistent")
			So(missingStr, ShouldEqual, "")

			missingFloat := Attribute[float64](payload, "data", 0, "missing_price")
			So(missingFloat, ShouldEqual, 0)

			val, ok := AttributeOk[string](payload, "channel")
			So(ok, ShouldBeTrue)
			So(val, ShouldEqual, "trade")

			_, ok = AttributeOk[string](payload, "missing_channel")
			So(ok, ShouldBeFalse)
		})
	})
}

func BenchmarkAttribute_Float(b *testing.B) {
	payload := []byte(`{"channel":"trade","data":[{"symbol":"BTC/USD","price":67500.50,"qty":1.25}]}`)

	b.ReportAllocs()
	for b.Loop() {
		_ = Attribute[float64](payload, "data", 0, "price")
	}
}

func BenchmarkAttribute_StringNumber(b *testing.B) {
	payload := []byte(`{"channel":"trade","data":[{"symbol":"BTC/USD","price":"67500.50","qty":1.25}]}`)

	b.ReportAllocs()
	for b.Loop() {
		_ = Attribute[float64](payload, "data", 0, "price")
	}
}
