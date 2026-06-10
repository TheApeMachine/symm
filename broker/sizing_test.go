package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
)

func TestSizeOrderEntry(t *testing.T) {
	Convey("Given quote cash and a mark", t, func() {
		viper.Set("trading.position_fraction", 0.2)

		action := &logic.Action{
			Type:     logic.ActionMarket,
			Side:     trading.Buy,
			Fraction: 0.25,
		}

		quantity, err := SizeOrder(action, 200, 0, 50_000)

		Convey("It should size from cash fraction and mark", func() {
			So(err, ShouldBeNil)
			So(quantity, ShouldAlmostEqual, 0.0002, 1e-9)
		})
	})
}

func TestSizeOrderExit(t *testing.T) {
	Convey("Given held inventory", t, func() {
		action := &logic.Action{
			Type:     logic.ActionSettlePosition,
			Side:     trading.Sell,
			Fraction: 1.0,
		}

		quantity, err := SizeOrder(action, 200, 0.5, 50_000)

		Convey("It should size from inventory fraction", func() {
			So(err, ShouldBeNil)
			So(quantity, ShouldEqual, 0.5)
		})
	})
}

func TestSizeOrderExitRequiresInventory(t *testing.T) {
	Convey("Given no inventory", t, func() {
		action := &logic.Action{
			Type:     logic.ActionSettlePosition,
			Side:     trading.Sell,
			Fraction: 1.0,
		}

		_, err := SizeOrder(action, 200, 0, 50_000)

		Convey("It should reject the exit", func() {
			So(err, ShouldEqual, ErrNoPosition)
		})
	})
}

func TestSizeOrderNegativeQuoteCash(t *testing.T) {
	Convey("Given negative quote cash", t, func() {
		viper.Set("trading.position_fraction", 0.2)

		action := &logic.Action{
			Type:     logic.ActionMarket,
			Side:     trading.Buy,
			Fraction: 0.25,
		}

		_, err := SizeOrder(action, -1, 0, 50_000)

		Convey("It should return a distinct error", func() {
			So(err, ShouldEqual, ErrNegativeQuoteCash)
		})
	})
}
