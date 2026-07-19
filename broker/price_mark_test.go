package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestMarkUsesLastTradeWhenBidMissing(t *testing.T) {
	Convey("Given a fill mark before the ticker book is warm", t, func() {
		price := NewPrice(nil)
		_ = price.RememberFee("BTC/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		})
		pair := &kraken.InstrumentPair{
			Symbol: "BTC/USD", Base: "BTC", Quote: "USD",
			QtyPrecision: 8, CostPrecision: 8,
		}
		holding := &types.Holding{
			Symbol:     "BTC/USD",
			Qty:        decimal.NewFromFloat64(1),
			EntryPrice: decimal.NewFromFloat64(50000),
			EntryFee:   decimal.NewFromFloat64(130),
			Mark:       decimal.NewFromFloat64(50000),
		}

		err := price.Mark(pair, holding)

		Convey("Then flatten-now PnL lands from the last trade mark", func() {
			So(err, ShouldBeNil)
			So(holding.PnL, ShouldNotBeNil)
			So(holding.PnL.Float64(), ShouldAlmostEqual, -260.0, 1e-8)
			So(holding.ReturnPct, ShouldNotBeNil)
		})
	})
}
