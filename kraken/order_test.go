package kraken

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

/*
TestNewMarketOrder proves an executable quantity reaches Kraken as an exact
JSON number rather than passing through binary floating-point arithmetic.
*/
func TestNewMarketOrder(t *testing.T) {
	Convey("Given a market quantity with more precision than float64 can retain", t, func() {
		quantity, err := decimal.NewFromString("0.123456789012345678")
		So(err, ShouldBeNil)
		order := NewMarketOrder("buy", quantity, "BTC/USD")

		Convey("It should preserve the literal quantity on the numeric wire", func() {
			encoded, marshalErr := order.MarshalJSON()

			So(marshalErr, ShouldBeNil)
			So(order.Params.OrderQty.String(), ShouldEqual, quantity.String())
			So(string(encoded), ShouldContainSubstring,
				`"order_qty":0.123456789012345678`)
			So(string(encoded), ShouldNotContainSubstring,
				`"order_qty":"0.123456789012345678"`)
		})
	})
}

/*
TestNewLimitOrder proves both monetary operands retain their exact decimal text
while satisfying Kraken's numeric limit-order schema.
*/
func TestNewLimitOrder(t *testing.T) {
	Convey("Given exact limit price and quantity decimals", t, func() {
		limitPrice, priceErr := decimal.NewFromString("43125.300")
		quantity, quantityErr := decimal.NewFromString("0.15000000")
		So(priceErr, ShouldBeNil)
		So(quantityErr, ShouldBeNil)
		order := NewLimitOrder("sell", limitPrice, quantity, "BTC/USD")

		Convey("It should emit both as unquoted JSON numbers", func() {
			encoded, err := order.MarshalJSON()

			So(err, ShouldBeNil)
			So(string(encoded), ShouldContainSubstring, `"limit_price":43125.300`)
			So(string(encoded), ShouldContainSubstring, `"order_qty":0.15000000`)
		})
	})
}

/*
BenchmarkMarketOrderMarshalJSON measures the exact-decimal live order encoding
path so preserving decimal text remains visible as an execution-path cost.
*/
func BenchmarkMarketOrderMarshalJSON(b *testing.B) {
	quantity, err := decimal.NewFromString("0.123456789012345678")

	if err != nil {
		b.Fatal(err)
	}

	order := NewMarketOrder("buy", quantity, "BTC/USD")
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if _, marshalErr := order.MarshalJSON(); marshalErr != nil {
			b.Fatal(marshalErr)
		}
	}
}
