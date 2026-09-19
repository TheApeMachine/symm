package derivatives_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic/derivatives"
)

func TestDerivatives(t *testing.T) {
	Convey("Given derivatives trade liquidation", t, func() {
		assemble := derivatives.NewTradeAssemble()
		liquidation := derivatives.NewLiquidation()
		buyPicker := derivatives.NewLiquidationBuy()
		sellPicker := derivatives.NewLiquidationSell()
		netPicker := derivatives.NewNetLiquidation()
		grossPicker := derivatives.NewGrossLiquidation()

		tick := map[string]any{
			"futures": map[string]any{
				"data": map[string]any{
					"symbol":    "PF_XBTUSD",
					"side":      "buy",
					"price":     50000.0,
					"qty":       2.0,
					"type":      "liquidation",
					"timestamp": int64(1700000000),
				},
			},
		}

		fill := assemble(tick)
		So(fill, ShouldNotBeNil)
		So(fill.Symbol, ShouldEqual, "PF_XBTUSD")
		So(fill.Price, ShouldEqual, 50000.0)

		reading := liquidation(fill)
		So(buyPicker(reading), ShouldEqual, 100000.0)
		So(sellPicker(reading), ShouldEqual, 0.0)
		So(netPicker(reading), ShouldEqual, 100000.0)
		So(grossPicker(reading), ShouldEqual, 100000.0)
	})

	Convey("Given derivatives ticker basis", t, func() {
		assemble := derivatives.NewAssemble()
		basis := derivatives.NewBasis()
		basisVal := derivatives.NewBasisValue()
		lastPrice := derivatives.NewDerivativePrice()
		refPrice := derivatives.NewReferencePrice()

		tick := map[string]any{
			"ticker": map[string]any{
				"data": map[string]any{
					"symbol":       "PF_XBTUSD",
					"last":         50500.0,
					"index":        50000.0,
					"openInterest": 1000.0,
					"timestamp":    int64(1700000000),
				},
			},
		}

		snap := assemble(tick)
		So(snap, ShouldNotBeNil)
		So(snap.Last, ShouldEqual, 50500.0)
		So(snap.Index, ShouldEqual, 50000.0)

		reading := basis(snap)
		So(lastPrice(reading), ShouldEqual, 50500.0)
		So(refPrice(reading), ShouldEqual, 50000.0)
		So(basisVal(reading), ShouldAlmostEqual, 0.01, 0.0001)
	})
}
