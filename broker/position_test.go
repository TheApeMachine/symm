package broker

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
)

func TestVirtualExploreCloseTeaches(t *testing.T) {
	Convey("VirtualExploreCloseTeaches", t, func() {
		Convey("reused paths always teach", func() {
			position := &Position{Virtual: true, PathFound: true}
			position.PnL = decimal.NewFromFloat64(-0.5)
			So(position.VirtualExploreCloseTeaches(), ShouldBeTrue)
		})

		Convey("new path teaches on a profitable close", func() {
			position := &Position{Virtual: true, PathFound: false}
			position.PnL = decimal.NewFromFloat64(0.01)
			So(position.VirtualExploreCloseTeaches(), ShouldBeTrue)
		})

		Convey("new path loss is ignored", func() {
			position := &Position{Virtual: true, PathFound: false}
			position.PnL = decimal.NewFromFloat64(-0.01)
			So(position.VirtualExploreCloseTeaches(), ShouldBeFalse)
		})
	})
}

func TestPositionVirtualExploreWin(t *testing.T) {
	Convey("VirtualExploreWin is profit at close only", t, func() {
		Convey("zero or positive PnL is a win", func() {
			position := &Position{Virtual: true}
			position.PnL = decimal.NewFromInt64(0)
			So(position.VirtualExploreWin(), ShouldBeTrue)
		})

		Convey("negative PnL at close is a loss", func() {
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

func TestVirtualSymmetricBookStop(t *testing.T) {
	Convey("virtual lots exit at twice the opening underwater return", t, func() {
		position := &Position{
			Virtual:    true,
			OpenPnLPct: -1.6,
		}
		position.OpenPnL = decimal.NewFromFloat64(-3.2)
		position.PnL = decimal.NewFromFloat64(-6.5)
		position.PnLPct = decimal.NewFromFloat64(-3.3)

		trigger, ok := position.ShouldExit()
		So(ok, ShouldBeTrue)
		So(trigger, ShouldEqual, "symmetric book stop")

		position.PnLPct = decimal.NewFromFloat64(-2.0)
		_, ok = position.ShouldExit()
		So(ok, ShouldBeFalse)
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
