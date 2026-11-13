package types

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

func TestExecutionSurface(t *testing.T) {
	Convey("Setup", t, func() {
		at := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
		sellable := decimal.RequireFromString("1.5")
		bid := decimal.RequireFromString("100.1")
		ask := decimal.RequireFromString("100.2")
		execQty := decimal.RequireFromString("1.5")
		gross := decimal.RequireFromString("150.15")
		vwap := decimal.RequireFromString("100.1")
		value := decimal.RequireFromString("149.55")
		floor := decimal.RequireFromString("1.5")

		Convey("When an ExecutionSurface is constructed", func() {
			surf := ExecutionSurface{
				Symbol:           "BTC/USD",
				At:               at,
				SellableQty:      &sellable,
				BestBid:          &bid,
				BestAsk:          &ask,
				ExecutableQty:    &execQty,
				Gross:            &gross,
				ExecutableVWAP:   &vwap,
				ExecutableValue:  &value,
				FloorCoverageQty: &floor,
				BookComplete:     true,
				FullyExecutable:  true,
			}

			Convey("liquidation facts are exact", func() {
				So(surf.Symbol, ShouldEqual, "BTC/USD")
				So(surf.At.Equal(at), ShouldBeTrue)
				So(surf.SellableQty.String(), ShouldEqual, "1.5")
				So(surf.BestBid.String(), ShouldEqual, "100.1")
				So(surf.BestAsk.String(), ShouldEqual, "100.2")
				So(surf.ExecutableQty.String(), ShouldEqual, "1.5")
				So(surf.Gross.String(), ShouldEqual, "150.15")
				So(surf.ExecutableVWAP.String(), ShouldEqual, "100.1")
				So(surf.ExecutableValue.String(), ShouldEqual, "149.55")
				So(surf.FloorCoverageQty.String(), ShouldEqual, "1.5")
				So(surf.BookComplete, ShouldBeTrue)
				So(surf.FullyExecutable, ShouldBeTrue)
			})
		})

		Convey("When book coverage is incomplete", func() {
			surf := ExecutionSurface{
				Symbol:          "ETH/USD",
				At:              at,
				BookComplete:    false,
				FullyExecutable: false,
			}

			Convey("nil depth fields and false flags are retained", func() {
				So(surf.Symbol, ShouldEqual, "ETH/USD")
				So(surf.SellableQty, ShouldBeNil)
				So(surf.BestBid, ShouldBeNil)
				So(surf.BestAsk, ShouldBeNil)
				So(surf.ExecutableQty, ShouldBeNil)
				So(surf.Gross, ShouldBeNil)
				So(surf.ExecutableVWAP, ShouldBeNil)
				So(surf.ExecutableValue, ShouldBeNil)
				So(surf.FloorCoverageQty, ShouldBeNil)
				So(surf.BookComplete, ShouldBeFalse)
				So(surf.FullyExecutable, ShouldBeFalse)
			})
		})
	})
}

func BenchmarkExecutionSurface(b *testing.B) {
	at := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	sellable := decimal.RequireFromString("1.5")
	bid := decimal.RequireFromString("100.1")
	ask := decimal.RequireFromString("100.2")

	b.ReportAllocs()

	for b.Loop() {
		surf := ExecutionSurface{
			Symbol: "BTC/USD", At: at,
			SellableQty: &sellable, BestBid: &bid, BestAsk: &ask,
			BookComplete: true, FullyExecutable: true,
		}
		_ = surf.Symbol
	}
}
