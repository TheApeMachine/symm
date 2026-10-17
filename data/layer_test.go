package data

import (
	"bytes"
	"errors"
	"io"
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

			val, err := AttributeOk[string](payload, "channel")
			So(err, ShouldBeNil)
			So(val, ShouldEqual, "trade")

			_, err = AttributeOk[string](payload, "missing_channel")
			So(errors.Is(err, ErrMissing), ShouldBeTrue)
		})

		Convey("AttributeOk decodes structured attributes from their raw JSON", func() {
			payload := []byte(`{"channel":"trade","data":[{"symbol":"BTC/USD","price":67500.5,"qty":1.25}]}`)
			trades, err := AttributeOk[[]sampleTrade](payload, "data")
			So(err, ShouldBeNil)
			So(trades, ShouldHaveLength, 1)
			So(trades[0].Symbol, ShouldEqual, "BTC/USD")
			So(trades[0].Price, ShouldEqual, 67500.5)

			first, err := AttributeOk[sampleTrade](payload, "data", 0)
			So(err, ShouldBeNil)
			So(first.Qty, ShouldEqual, 1.25)
		})

		Convey("AttributeOk returns an error, never a silent zero, for an undecodable attribute", func() {
			trades, err := AttributeOk[[]sampleTrade](payload, "channel")
			So(err, ShouldNotBeNil)
			So(errors.Is(err, ErrMissing), ShouldBeFalse)
			So(err.Error(), ShouldContainSubstring, "not decodable as")
			So(trades, ShouldBeNil)
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

func TestSignals(t *testing.T) {
	Convey("Given a stored tick with signal layers for two symbols", t, func() {
		tick := &Value{Origin: TickOrigin, Role: TickRole, Scope: TickScope, Epoch: 7, Tick: 42}
		tick.FormatPrefix()
		tick.AppendJSON([]byte(`{"bid":"1"}`), "price/BTC/USD")
		AppendSignalVector(tick, "cvd", "BTC/USD", []float64{1.5, -0.25}, "gross_notional", "signed_net_fraction", "window:5m")
		AppendSignalVector(tick, "hawkes", "ETH/USD", []float64{0.75}, "intensity")

		Convey("Its key is the one Prefix derives", func() {
			So(tick.Prefix, ShouldEqual, TickPrefix(7)+"0000000000000000042.bin")
			So(tick.Prefix, ShouldStartWith, "workspace/tick/aggregate/0000000000000000007/")
		})

		Convey("Signals attributes every vector layer after a storage round trip", func() {
			var buf bytes.Buffer
			_, err := io.Copy(&buf, tick)
			So(err, ShouldBeNil)

			stored := New()
			_, err = stored.Write(buf.Bytes())
			So(err, ShouldBeNil)

			signals := make([]Signal, 0)
			for signal, err := range Signals(stored) {
				So(err, ShouldBeNil)
				signals = append(signals, signal)
			}

			So(signals, ShouldHaveLength, 2)
			So(signals[0].Source, ShouldEqual, "cvd")
			So(signals[0].Symbol, ShouldEqual, "BTC/USD")
			So(signals[0].Labels, ShouldResemble, []string{"gross_notional", "signed_net_fraction"})
			So(signals[0].Values, ShouldResemble, []float64{1.5, -0.25})
			So(signals[0].Tags, ShouldResemble, []string{"window:5m"})
			So(signals[1].Source, ShouldEqual, "hawkes")
			So(signals[1].Symbol, ShouldEqual, "ETH/USD")

			value, err := signals[0].Metric("signed_net_fraction")
			So(err, ShouldBeNil)
			So(value, ShouldEqual, -0.25)

			_, err = signals[0].Metric("absent")
			So(err, ShouldNotBeNil)
		})

		Convey("The index is the canonical signal index, free of the tick's key", func() {
			So(tick.Index[1], ShouldEqual, "signal/cvd/gross_notional/signed_net_fraction/window:5m/symbol:BTC/USD")
			So(tick.Index[1], ShouldEqual, SignalIndex("cvd", "BTC/USD", "gross_notional", "signed_net_fraction", "window:5m"))
			So(tick.Index[1], ShouldNotContainSubstring, tick.Prefix)
		})

		Convey("Signal layers are not merged into Metrics", func() {
			So(Metrics(tick), ShouldBeEmpty)
		})

		Convey("A tag containing /symbol: stays a tag; the last symbol tag names the symbol", func() {
			AppendSignalVector(tick, "cvd", "SOL/USD", []float64{2}, "gross_notional", "ref:x/symbol:y")

			var last Signal
			for signal, err := range Signals(tick) {
				So(err, ShouldBeNil)
				last = signal
			}

			So(last.Symbol, ShouldEqual, "SOL/USD")
			So(last.Labels, ShouldResemble, []string{"gross_notional"})
			So(last.Tags, ShouldResemble, []string{"ref:x", "symbol:y"})
		})

		Convey("A signal layer without a symbol tag is an error", func() {
			tick.claim(vectorBytes([]float64{1}), "signal/cvd/gross_notional")

			var last error
			for _, err := range Signals(tick) {
				last = err
			}

			So(last, ShouldNotBeNil)
		})

		Convey("A layer with more values than labels is an error", func() {
			AppendSignalVector(tick, "cvd", "BTC/USD", []float64{1, 2})

			var last error
			for _, err := range Signals(tick) {
				last = err
			}

			So(last, ShouldNotBeNil)
		})
	})
}
