package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

func TestSizeOrderEntry(t *testing.T) {
	Convey("Given quote cash and a mark", t, func() {
		risk := RiskContext{
			PositionFraction: 0.2,
		}

		action := &logic.Action{
			Type:     logic.ActionMarket,
			Side:     trading.Buy,
			Fraction: 0.25,
		}

		quantity, err := SizeOrder(action, risk, 1.0, 200, 0, 50_000, nil)

		Convey("It should size from cash fraction and mark", func() {
			So(err, ShouldBeNil)
			So(quantity, ShouldAlmostEqual, 0.0002, 1e-9)
		})
	})
}

func TestSizeOrderEntryUsesRegimeScale(t *testing.T) {
	Convey("Given a choppy regime scale", t, func() {
		risk := RiskContext{
			PositionFraction: 0.2,
		}

		action := &logic.Action{
			Type:     logic.ActionMarket,
			Side:     trading.Buy,
			Fraction: 0.25,
		}

		quantity, err := SizeOrder(action, risk, 0.5, 200, 0, 50_000, nil)

		Convey("It should scale entry notional", func() {
			So(err, ShouldBeNil)
			So(quantity, ShouldAlmostEqual, 0.0001, 1e-9)
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

		quantity, err := SizeOrder(action, RiskContext{}, 1.0, 200, 0.5, 50_000, nil)

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

		_, err := SizeOrder(action, RiskContext{}, 1.0, 200, 0, 50_000, nil)

		Convey("It should reject the exit", func() {
			So(err, ShouldEqual, ErrNoPosition)
		})
	})
}

func TestSizeOrderNegativeQuoteCash(t *testing.T) {
	Convey("Given negative quote cash", t, func() {
		risk := RiskContext{
			PositionFraction: 0.2,
		}

		action := &logic.Action{
			Type:     logic.ActionMarket,
			Side:     trading.Buy,
			Fraction: 0.25,
		}

		_, err := SizeOrder(action, risk, 1.0, -1, 0, 50_000, nil)

		Convey("It should return a distinct error", func() {
			So(err, ShouldEqual, ErrNegativeQuoteCash)
		})
	})
}

func TestRiskContextEntryScaleForRegime(t *testing.T) {
	Convey("Given configured regime scales", t, func() {
		viper.Set("trading.replay.choppy_size_scale", 0.5)
		viper.Set("trading.replay.bearish_size_scale", 0.35)

		risk := LoadRiskContext()

		Convey("It should pick choppy when choppiness dominates", func() {
			scale := risk.EntryScaleForRegime(market.RegimeStrengths{
				Choppiness: 0.9,
				Bearish:    0.2,
				Trend:      0.1,
			})

			So(scale, ShouldEqual, 0.5)
		})

		Convey("It should pick bearish when bearish dominates", func() {
			scale := risk.EntryScaleForRegime(market.RegimeStrengths{
				Bearish: 0.8,
				Trend:   0.2,
			})

			So(scale, ShouldEqual, 0.35)
		})
	})
}

func TestNormalizeAsset(t *testing.T) {
	Convey("Given Kraken asset codes", t, func() {
		Convey("It should normalize prefixed assets", func() {
			So(NormalizeAsset("ZUSD"), ShouldEqual, "USD")
			So(NormalizeAsset("ZEUR"), ShouldEqual, "EUR")
		})
	})
}

func BenchmarkSizeOrderEntry(b *testing.B) {
	risk := RiskContext{
		PositionFraction: 0.2,
	}

	action := &logic.Action{
		Type:     logic.ActionMarket,
		Side:     trading.Buy,
		Fraction: 0.25,
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = SizeOrder(action, risk, 1.0, 200, 0, 50_000, nil)
	}
}
