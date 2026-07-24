package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func lrcPair() kraken.InstrumentPair {
	return kraken.InstrumentPair{
		Symbol:        "LRC/USD",
		Base:          "LRC",
		Quote:         "USD",
		QtyMin:        decimal.NewFromInt64(50),
		QtyIncrement:  decimal.NewFromInt64(1),
		QtyPrecision:  0,
		CostMin:       decimal.NewFromFloat64(0.5),
		CostPrecision: 5,
	}
}

func wireLrcPrice() (*Price, kraken.InstrumentPair) {
	price := NewPrice(nil)
	_ = price.RememberFee("LRC/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.26),
	})
	price.TickerAck(kraken.NewTicker([]byte(
		`{"channel":"ticker","type":"update","data":[{` +
			`"symbol":"LRC/USD","last":"0.05","bid":"0.04999","ask":"0.05"}]}`,
	)))

	return price, lrcPair()
}

/*
TestPriceQuantity proves mandatory taker fees and integer-lattice binary search
return the largest executable lot within budget.
*/
func TestPriceQuantity(t *testing.T) {
	Convey("Given a funded LRC/USD pair with cached ask and taker fee", t, func() {
		price, pair := wireLrcPrice()
		budget := decimal.NewFromFloat64(999.99324)

		Convey("Quantity returns the max lattice lot whose taker cost fits", func() {
			quantity, err := price.Quantity(&pair, budget)
			So(err, ShouldBeNil)
			So(quantity, ShouldNotBeNil)
			So(quantity.Float64(), ShouldEqual, 19948)

			cost, costErr := price.Taker(&pair, quantity)
			So(costErr, ShouldBeNil)
			So(cost.Cmp(budget), ShouldBeLessThanOrEqualTo, 0)

			larger := quantity.Copy().Add(decimal.NewFromInt64(1))
			largerCost, largerErr := price.Taker(&pair, larger)
			So(largerErr, ShouldBeNil)
			So(largerCost.Cmp(budget), ShouldBeGreaterThan, 0)
		})

		Convey("Quantity rejects when the taker fee tier is missing", func() {
			missingFee := NewPrice(nil)
			missingFee.TickerAck(kraken.NewTicker([]byte(
				`{"channel":"ticker","type":"update","data":[{` +
					`"symbol":"LRC/USD","last":"0.05","bid":"0.04999","ask":"0.05"}]}`,
			)))

			_, err := missingFee.Quantity(&pair, budget)
			So(err, ShouldNotBeNil)
		})

		Convey("Quantity rejects when budget is below instrument minimum cost", func() {
			_, err := price.Quantity(&pair, decimal.NewFromFloat64(1))
			So(err, ShouldNotBeNil)
		})
	})
}

/*
BenchmarkPriceQuantity measures integer-lattice sizing on the enter hot path.
*/
func BenchmarkPriceQuantity(b *testing.B) {
	price, pair := wireLrcPrice()
	budget := decimal.NewFromFloat64(999.99324)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = price.Quantity(&pair, budget)
	}
}
