package broker_test

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
)

func TestPosition(t *testing.T) {
	Convey("Given a new Position", t, func() {
		position := broker.NewPosition()
		So(position, ShouldNotBeNil)
		So(position.Closed(), ShouldBeTrue)

		Convey("When Enter is called with a quantity", func() {
			qty := decimal.NewFromFloat64(1.5)
			err := position.Enter("BTC/USD", qty)
			So(err, ShouldBeNil)
			So(position.Qty().Cmp(qty), ShouldEqual, 0)
			So(position.Closed(), ShouldBeFalse)

			Convey("When a buy trade execution fill is applied", func() {
				buyPrice := decimal.NewFromFloat64(60000)
				fee := decimal.NewFromFloat64(0.2)

				position.Apply(kraken.ExecutionData{
					Symbol:      "BTC/USD",
					Side:        "buy",
					ExecType:    "trade",
					LastQty:     qty,
					LastPrice:   buyPrice,
					FeeUsdEquiv: fee,
				})

				So(position.EntryPrice().Cmp(buyPrice), ShouldEqual, 0)
				So(position.Closed(), ShouldBeFalse)

				Convey("When Exit is called and a sell trade execution fill is applied", func() {
					exitErr := position.Exit("BTC/USD")
					So(exitErr, ShouldBeNil)

					sellPrice := decimal.NewFromFloat64(62000)

					position.Apply(kraken.ExecutionData{
						Symbol:      "BTC/USD",
						Side:        "sell",
						ExecType:    "trade",
						LastQty:     qty,
						LastPrice:   sellPrice,
						FeeUsdEquiv: fee,
					})

					So(position.ExitPrice().Cmp(sellPrice), ShouldEqual, 0)
					So(position.Closed(), ShouldBeTrue)
				})
			})
		})
	})
}
