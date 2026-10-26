package broker

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
)

func TestPositionVirtualExploreWin(t *testing.T) {
	Convey("VirtualExploreWin on a virtual lot", t, func() {
		Convey("non-negative PnL is a win", func() {
			position := &Position{Virtual: true}
			position.PnL = decimal.NewFromInt64(0)
			So(position.VirtualExploreWin(), ShouldBeTrue)
		})

		Convey("peak open PnL is a win when exit PnL dipped", func() {
			position := &Position{
				Virtual:        true,
				PeakOpenPnLPct: 0.01,
			}
			position.PnL = decimal.NewFromFloat64(-0.01)
			So(position.VirtualExploreWin(), ShouldBeTrue)
		})

		Convey("a loss with no peak is not a win", func() {
			position := &Position{Virtual: true}
			position.PnL = decimal.NewFromFloat64(-0.1)
			So(position.VirtualExploreWin(), ShouldBeFalse)
		})
	})
}

func TestPositionTrySettleExplore(t *testing.T) {
	Convey("TrySettleExplore records learning once per lot", t, func() {
		position := NewPosition("BTC/USD", "R07.R00", true, nil)
		So(position.TrySettleExplore(), ShouldBeTrue)
		So(position.TrySettleExplore(), ShouldBeFalse)
	})
}

func TestPositionEnterExitPath(t *testing.T) {
	Convey("Enter and Exit require a path", t, func() {
		position := NewPosition("BTC/USD", "", true, nil)

		Convey("Enter rejects a blank path", func() {
			_, err := position.Enter("")
			So(err, ShouldNotBeNil)
			So(position.OpenedAt.IsZero(), ShouldBeTrue)
		})

		Convey("Exit rejects a blank path", func() {
			position.ExitOrders = []*spot.OrderRequest{{}}
			position.OpenedAt = time.Now().UTC()

			_, err := position.Exit("")
			So(err, ShouldNotBeNil)
			So(position.ClosedAt, ShouldBeNil)
			So(position.ExitPath, ShouldEqual, "")
		})
	})
}
