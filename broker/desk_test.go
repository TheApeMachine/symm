package broker

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
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
			convey.So(triggers.Price, convey.ShouldAlmostEqual, -0.25, 1e-9)
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
			convey.So(action.Offset, convey.ShouldAlmostEqual, 0.0025, 1e-9)
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

		convey.Convey("ResetHalt should clear the circuit breaker", func() {
			desk, err := NewDeskWithAllCaches(ctx, nil, quotes, stress, rules)

			convey.So(err, convey.ShouldBeNil)
			desk.TripHalt()
			convey.So(desk.Halted(), convey.ShouldBeTrue)
			desk.ResetHalt()
			convey.So(desk.Halted(), convey.ShouldBeFalse)
		})

		convey.Convey("Halted should auto-reset after the cool-down elapses", func() {
			desk, err := NewDeskWithAllCaches(ctx, nil, quotes, stress, rules)

			convey.So(err, convey.ShouldBeNil)
			desk.haltCooldown = time.Millisecond
			desk.TripHalt()
			convey.So(desk.Halted(), convey.ShouldBeTrue)
			time.Sleep(2 * time.Millisecond)
			convey.So(desk.Halted(), convey.ShouldBeFalse)
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

func TestDeskAddOrderPreparesEntryMinimum(t *testing.T) {
	testconfig.Load(t)

	convey.Convey("Given a desk entry below Kraken minimum quantity", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 4, qpool.NewConfig())
		defer pool.Close()

		private, err := qpool.NewBroadcastGroup(ctx, "kraken:private", 10*time.Millisecond)
		if err != nil {
			t.Fatal("expected private broadcast group")
		}
		privateSub := private.Subscribe("test:desk-entry-minimum", 4)
		quotes := NewQuoteCache(ctx, nil)
		quotes.InstallQuoteForTest(Quote{
			Symbol:    "FXS/EUR",
			Bid:       4.35,
			Ask:       4.36,
			Last:      4.36,
			UpdatedAt: time.Now(),
		})

		stress := NewStressCache(ctx, nil)
		rules := NewInstrumentRulesCache(ctx)
		rules.InstallPairForTest(market.InstrumentPair{
			Symbol:       "FXS/EUR",
			QtyIncrement: 0.00000001,
			QtyMin:       12,
			CostMin:      10,
		})

		desk, err := NewDeskWithAllCaches(ctx, pool, quotes, stress, rules)
		convey.So(err, convey.ShouldBeNil)
		defer desk.Close()

		resolved, err := desk.AddOrder(reasoning.Action{
			Type:     reasoning.ActionLimit,
			Symbol:   "FXS/EUR",
			Side:     trading.Buy,
			Quantity: 11.42752815,
			Price:    4.34,
		})

		convey.Convey("It should submit the aligned exchange minimum", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(resolved.Quantity, convey.ShouldEqual, 12)

			waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
			defer waitCancel()

			message, err := privateSub.Wait(waitCtx)
			if err != nil {
				t.Fatal("expected add_order frame")
			}

			frame, ok := message.Value.(map[string]any)
			convey.So(ok, convey.ShouldBeTrue)
			params, ok := frame["params"].(trading.AddParams)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(params.OrderQty, convey.ShouldEqual, 12)
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
