package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
TestMarkUsesLastTradeWhenBidMissing proves flatten-now PnL can use the last
trade mark before the ticker book is warm.
*/
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

/*
TestGeometryMarkOmitsBidFallback proves stop geometry never copies bid Mark when
mid and last are absent — Present freezes instead of inventing a spread breach.
*/
func TestGeometryMarkOmitsBidFallback(t *testing.T) {
	Convey("Given a bid-only ticker after an ask entry", t, func() {
		price := NewPrice(nil)
		_ = price.RememberFee("VANRY/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		})
		pair := &kraken.InstrumentPair{
			Symbol: "VANRY/USD", Base: "VANRY", Quote: "USD",
			QtyPrecision: 8, CostPrecision: 8,
		}
		holding := &types.Holding{
			Symbol:     "VANRY/USD",
			Qty:        decimal.NewFromFloat64(100),
			EntryPrice: decimal.NewFromFloat64(0.01),
			EntryFee:   decimal.NewFromFloat64(0.000026),
		}

		price.TickerAck([]byte(
			`{"channel":"ticker","type":"update","data":[{` +
				`"symbol":"VANRY/USD","last":"0","bid":"0.0096","ask":"0"}]}`,
		))
		err := price.Mark(pair, holding)

		Convey("Then bid Mark is set for PnL but StopMark stays nil", func() {
			So(err, ShouldBeNil)
			So(holding.Mark, ShouldNotBeNil)
			So(holding.StopMark, ShouldBeNil)
		})
	})

	Convey("Given a warm two-sided ticker", t, func() {
		price := NewPrice(nil)
		_ = price.RememberFee("VANRY/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		})
		pair := &kraken.InstrumentPair{
			Symbol: "VANRY/USD", Base: "VANRY", Quote: "USD",
			QtyPrecision: 8, CostPrecision: 8,
		}
		holding := &types.Holding{
			Symbol:     "VANRY/USD",
			Qty:        decimal.NewFromFloat64(100),
			EntryPrice: decimal.NewFromFloat64(0.01),
			EntryFee:   decimal.NewFromFloat64(0.000026),
		}

		price.TickerAck([]byte(
			`{"channel":"ticker","type":"update","data":[{` +
				`"symbol":"VANRY/USD","last":"0.01","bid":"0.0098","ask":"0.0102"}]}`,
		))
		err := price.Mark(pair, holding)

		Convey("Then StopMark is touch mid", func() {
			So(err, ShouldBeNil)
			So(holding.StopMark, ShouldNotBeNil)
			So(holding.StopMark.Float64(), ShouldAlmostEqual, 0.01, 1e-12)
		})
	})
}
