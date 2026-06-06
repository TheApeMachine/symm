package broker

import (
	"strings"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestDesk_NextClOrdID(t *testing.T) {
	convey.Convey("Given a desk", t, func() {
		desk := &Desk{}

		convey.Convey("When NextClOrdID is called", func() {
			clOrdID := desk.NextClOrdID()

			convey.Convey("It should return a client order id with the desk prefix", func() {
				convey.So(clOrdID, convey.ShouldStartWith, "s")
				convey.So(len(clOrdID), convey.ShouldBeGreaterThan, 1)
			})
		})

		convey.Convey("When NextClOrdID is called twice", func() {
			first := desk.NextClOrdID()
			second := desk.NextClOrdID()

			convey.Convey("It should return distinct ids", func() {
				convey.So(first, convey.ShouldNotEqual, second)
				convey.So(strings.HasPrefix(first, "s"), convey.ShouldBeTrue)
				convey.So(strings.HasPrefix(second, "s"), convey.ShouldBeTrue)
			})
		})
	})
}

func BenchmarkDesk_NextClOrdID(b *testing.B) {
	desk := &Desk{}

	for b.Loop() {
		_ = desk.NextClOrdID()
	}
}

func TestDeskTriggersFor(t *testing.T) {
	testconfig.Load(t)

	convey.Convey("Given a trailing stop without a playbook offset", t, func() {
		desk := &Desk{}
		action := reasoning.Action{
			Type:   reasoning.ActionTrailingStop,
			Symbol: "BTC/EUR",
			Side:   trading.Sell,
		}

		convey.Convey("It should derive the Kraken percent from realized volatility", func() {
			triggers, err := desk.triggersFor(action, Quote{
				Symbol:     "BTC/EUR",
				Volatility: 0.001,
			})

			convey.So(err, convey.ShouldBeNil)
			convey.So(triggers.PriceType, convey.ShouldEqual, "pct")
			convey.So(triggers.Price, convey.ShouldAlmostEqual, -0.3, 1e-9)
		})

		convey.Convey("It should reject a dynamic trail without realized volatility", func() {
			triggers, err := desk.triggersFor(action, Quote{Symbol: "BTC/EUR"})

			convey.So(triggers, convey.ShouldBeNil)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "realized volatility")
		})

		convey.Convey("It should honor an explicit playbook offset", func() {
			action.Offset = 0.02
			triggers, err := desk.triggersFor(action, Quote{Symbol: "BTC/EUR"})

			convey.So(err, convey.ShouldBeNil)
			convey.So(triggers.Price, convey.ShouldAlmostEqual, -2, 1e-9)
		})
	})
}

func TestDeskResolveAction(t *testing.T) {
	testconfig.Load(t)

	convey.Convey("Given a desk with quote and stress caches", t, func() {
		quotes := NewQuoteCache(t.Context(), nil)
		quotes.InstallQuoteForTest(Quote{
			Symbol:     "BTC/EUR",
			Bid:        99,
			Ask:        100,
			Last:       100,
			Volatility: 0.001,
		})

		stress := NewStressCache(t.Context(), nil)
		stress.InstallStressForTest("BTC/EUR", SymbolStress{
			HawkesCategory: types.CategorySaturation,
			HawkesSNR:      1.0,
		})

		desk := &Desk{quotes: quotes, stress: stress}

		convey.Convey("It should scale entry quantity from hostile stress", func() {
			action, err := desk.ResolveAction(reasoning.Action{
				Type:     reasoning.ActionMarket,
				Symbol:   "BTC/EUR",
				Side:     trading.Buy,
				Quantity: 10,
			})

			convey.So(err, convey.ShouldBeNil)
			convey.So(action.Quantity, convey.ShouldAlmostEqual, 5, 1e-9)
		})

		convey.Convey("It should resolve dynamic trailing offsets before submission", func() {
			action, err := desk.ResolveAction(reasoning.Action{
				Type:   reasoning.ActionTrailingStop,
				Symbol: "BTC/EUR",
				Side:   trading.Sell,
			})

			convey.So(err, convey.ShouldBeNil)
			convey.So(action.Offset, convey.ShouldAlmostEqual, 0.003, 1e-9)
		})
	})
}

func TestNewDeskWithCaches(t *testing.T) {
	convey.Convey("Given desk dependencies", t, func() {
		ctx := t.Context()
		quotes := NewQuoteCache(ctx, nil)
		stress := NewStressCache(ctx, nil)
		rules := NewInstrumentRulesCache(ctx)

		convey.Convey("It should reject missing caches", func() {
			_, err := NewDeskWithAllCaches(ctx, nil, nil, stress, rules)

			convey.So(err, convey.ShouldNotBeNil)

			_, err = NewDeskWithAllCaches(ctx, nil, quotes, nil, rules)

			convey.So(err, convey.ShouldNotBeNil)

			_, err = NewDeskWithAllCaches(ctx, nil, quotes, stress, nil)

			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("It should construct a desk with explicit caches", func() {
			desk, err := NewDeskWithAllCaches(ctx, nil, quotes, stress, rules)

			convey.So(err, convey.ShouldBeNil)
			convey.So(desk, convey.ShouldNotBeNil)
			convey.So(desk.Halted(), convey.ShouldBeFalse)
			convey.So(desk.Close(), convey.ShouldBeNil)
		})

		convey.Convey("TripHalt should latch the circuit breaker", func() {
			desk, err := NewDeskWithAllCaches(ctx, nil, quotes, stress, rules)

			convey.So(err, convey.ShouldBeNil)
			desk.TripHalt()
			convey.So(desk.Halted(), convey.ShouldBeTrue)
			desk.TripHalt()
			convey.So(desk.Halted(), convey.ShouldBeTrue)
		})
	})
}

func TestDeskPrepareInstrumentOrder(t *testing.T) {
	convey.Convey("Given a desk with instrument rules", t, func() {
		rules := NewInstrumentRulesCache(t.Context())
		rules.InstallPairForTest(market.InstrumentPair{
			Symbol:       "LTC/EUR",
			QtyIncrement: 0.00000001,
			QtyMin:       0.00000001,
			CostMin:      0.01,
		})

		symbolStress := SymbolStress{
			HawkesCategory: types.CategorySaturation,
			HawkesSNR:      1,
		}

		desk := &Desk{rules: rules}

		convey.Convey("It should round stress-sized quantity before instrument validation", func() {
			action := reasoning.Action{
				Type:     reasoning.ActionLimit,
				Symbol:   "LTC/EUR",
				Quantity: 50.0 / 94.523,
			}

			resolved, err := resolveAction(action, Quote{Symbol: "LTC/EUR"}, symbolStress)

			convey.So(err, convey.ShouldBeNil)
			convey.So(
				isAligned(resolved.Quantity, 0.00000001),
				convey.ShouldBeFalse,
			)

			err = rules.ValidateOrder(
				resolved.Symbol,
				resolved.Quantity,
				resolved.Price,
				trading.Limit,
			)

			convey.So(err, convey.ShouldNotBeNil)

			alignedQty, _, err := desk.rules.PrepareOrder(
				resolved.Symbol,
				resolved.Quantity,
				resolved.Price,
				trading.Limit,
			)

			convey.So(err, convey.ShouldBeNil)
			convey.So(isAligned(alignedQty, 0.00000001), convey.ShouldBeTrue)
		})
	})
}

func TestDeskTriggerOffset(t *testing.T) {
	testconfig.Load(t)

	convey.Convey("Given configured exit offsets", t, func() {
		quote := Quote{Symbol: "BTC/EUR", Volatility: 0.001}

		convey.Convey("It should resolve stop-loss from config", func() {
			offset, err := triggerOffset(
				reasoning.Action{Type: reasoning.ActionStopLoss},
				quote,
			)

			convey.So(err, convey.ShouldBeNil)
			convey.So(offset, convey.ShouldBeGreaterThan, 0)
		})

		convey.Convey("It should resolve take-profit from config", func() {
			offset, err := triggerOffset(
				reasoning.Action{Type: reasoning.ActionTakeProfit},
				quote,
			)

			convey.So(err, convey.ShouldBeNil)
			convey.So(offset, convey.ShouldBeGreaterThan, 0)
		})

		convey.Convey("It should honor an explicit playbook offset", func() {
			offset, err := triggerOffset(
				reasoning.Action{Type: reasoning.ActionStopLoss, Offset: 0.015},
				quote,
			)

			convey.So(err, convey.ShouldBeNil)
			convey.So(offset, convey.ShouldAlmostEqual, 0.015, 1e-9)
		})
	})
}
