package trader_test

import (
	"context"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

/* TestCryptoTick proves current regimes and rejected frames cross production intact. */
func TestCryptoTick(t *testing.T) {
	proofs := []struct {
		name  string
		state tests.MarketState
		steps int
	}{
		{"fast pump", tests.MarketStateFastPump, 12},
		{"slow pump", tests.MarketStateSlowPump, 20},
		{"fast dump", tests.MarketStateFastDump, 12},
		{"slow dump", tests.MarketStateSlowDump, 20},
		{"volume absorption", tests.MarketStateVolumeAbsorption, 12},
		{"low-volume lift", tests.MarketStateLowVolumeLift, 12},
		{"spread compression", tests.MarketStateSpreadCompression, 12},
		{"thin liquidity", tests.MarketStateThinLiquidity, 1},
		{"loaded liquidity", tests.MarketStateLoadedLiquidity, 18},
		{"liquidity retreat", tests.MarketStateLiquidityRetreat, 1},
		{"spoof liquidity", tests.MarketStateSpoofLiquidity, 1},
		{"depth thinning", tests.MarketStateDepthThinning, 1},
		{"slow-cadence lift", tests.MarketStateSlowCadenceLift, 12},
		{"small-displacement lift", tests.MarketStateSmallLift, 12},
		{"spread control", tests.MarketStateSpreadControl, 12},
		{"leader follower", tests.MarketStateLeaderFollower, 20},
		{"adverse divergence", tests.MarketStateAdverseDivergence, 12},
	}

	Convey("Given one production stack spanning every simulated market regime", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})

		tick := int64(0)
		seen := map[types.SourceType]bool{}
		lastAt := map[string]time.Time{}
		consume := func() error {
			thesis, err := wired.Crypto.Tick()

			if err != nil {
				return err
			}

			tick++
			So(thesis, ShouldNotBeNil)
			So(thesis.Tick, ShouldEqual, tick)
			So(thesis.At, ShouldResemble, market.Now())
			So(thesis.Incomplete(), ShouldBeFalse)
			So(thesis.Measurements, ShouldNotBeEmpty)

			current := false

			for _, measurement := range thesis.Measurements {
				if measurement == nil {
					return fmt.Errorf("trader: nil measurement at tick %d", tick)
				}

				if err := measurement.ValidateStruct(); err != nil {
					return err
				}

				if measurement.At.After(market.Now()) {
					return fmt.Errorf(
						"trader: %s/%s measurement is ahead of cut %s",
						measurement.Source,
						measurement.Metric,
						market.Now(),
					)
				}

				identity := string(measurement.Source) + "\x00" +
					string(measurement.Metric) + "\x00" + measurement.Symbol + "\x00" +
					measurement.Peer + "\x00" + string(measurement.Side)
				priorAt, observed := lastAt[identity]

				if observed && measurement.At.Before(priorAt) {
					return fmt.Errorf(
						"trader: %s regressed from %s to %s",
						identity,
						priorAt,
						measurement.At,
					)
				}

				lastAt[identity] = measurement.At
				seen[measurement.Source] = true
				current = current || measurement.At.Equal(market.Now())
			}

			if !current {
				return fmt.Errorf("trader: tick %d has no current-cut measurement", tick)
			}

			for _, decision := range thesis.Decisions {
				switch decision.Action {
				case types.ActionEnter, types.ActionHold, types.ActionExit,
					types.ActionNothing:
				default:
					return fmt.Errorf(
						"trader: invalid %q decision for %s",
						decision.Action,
						decision.Symbol,
					)
				}
			}

			return nil
		}
		step := tests.MarketStep{
			Advance: time.Second,
			Actions: []tests.MarketAction{
				{Kind: tests.MarketTrade, Symbol: market.Symbols[0], Side: "buy", Qty: 1},
				{Kind: tests.MarketRefill, Symbol: market.Symbols[0], Side: "sell", Qty: 1},
			},
		}

		Convey("Every cut completes once, at its exact event time", func() {
			So(market.Warmup(consume), ShouldBeNil)
			So(tick, ShouldEqual, int64(16))

			for _, proof := range proofs {
				before := tick
				So(market.Transition(proof.state, consume), ShouldBeNil)
				So(tick-before, ShouldEqual, int64(proof.steps))
			}

			So(tick, ShouldEqual, int64(206))
			So(seen, ShouldResemble, map[types.SourceType]bool{
				types.SourceCorrelation: true,
				types.SourceCVD:         true,
				types.SourceDepthFlow:   true,
				types.SourceExhaustion:  true,
				types.SourceHawkes:      true,
				types.SourceLeadLag:     true,
				types.SourceLiquidity:   true,
				types.SourcePumpDump:    true,
				types.SourceSentiment:   true,
				types.SourceToxicity:    true,
			})
			So(wired.Crypto.Close(), ShouldBeNil)
			So(wired.Crypto.Close(), ShouldBeNil)
		})

		Convey("A rejected signal frame marks the complete cut unusable for entry", func() {
			So(market.Apply(step, func() error { return nil }), ShouldBeNil)
			thesis, err := wired.Crypto.Tick()
			So(err, ShouldBeNil)
			symbol := market.Symbols[0]
			thesis.Holdings.Store(symbol,
				types.NewHolding(t.Context(), symbol, decimal.NewFromInt64(1)))
			thesis.Decisions = append(thesis.Decisions, types.Decision{
				Action: types.ActionEnter, Symbol: symbol, At: thesis.At,
			})
			thesis.NoteLifecycle(symbol, types.LifecycleEntrySelected, thesis.At)
			So(market.Apply(step, func() error { return nil }), ShouldBeNil)
			malformed := fmt.Appendf(nil,
				`{"channel":"book","type":"update","data":[{"symbol":"%s",`+
					`"bids":[{"price":"99.99","qty":-1}],`+
					`"asks":[{"price":"100.01","qty":1}],"timestamp":"%s"}]}`,
				market.Symbols[0], market.Now().Format(time.RFC3339Nano),
			)
			So(market.Public.Publish("book", malformed), ShouldBeNil)
			So(market.Public.Drain(), ShouldBeNil)

			thesis, err = wired.Crypto.Tick()
			So(err, ShouldBeNil)
			So(thesis.Incomplete(), ShouldBeTrue)
			So(thesis.Decisions, ShouldHaveLength, 2)
			So(thesis.Decisions[0].Action, ShouldEqual, types.ActionEnter)
			So(thesis.Decisions[1].Action, ShouldEqual, types.ActionNothing)
			So(thesis.Decisions[1].Cause, ShouldEqual, "measure_incomplete")
			So(thesis.Decisions[1].Reason, ShouldEqual,
				"accumulated evidence is marked incomplete; refuse fresh enters")
			phase, found := thesis.Lifecycle.Load(symbol)
			So(found, ShouldBeTrue)
			So(phase, ShouldEqual, types.LifecycleEntrySelected)
			_, found = wired.Desk.Position(symbol)
			So(found, ShouldBeFalse)
		})
	})
}

/* TestCryptoRun proves bounded cadence recovery and joined runtime shutdown. */
func TestCryptoRun(t *testing.T) {
	Convey("Given a production runtime blocked on its configured idle budget", t, func(c C) {
		synctest.Test(t, func(t *testing.T) {
			market := tests.NewMarket(t.Context(), 3)
			wired, err := stack.NewBooter(t.Context()).Test(market)
			c.So(err, ShouldBeNil)
			defer market.Close()
			defer func() { c.So(wired.Close(), ShouldBeNil) }()

			c.So(market.Warmup(tests.Consume(wired.Crypto.Tick)), ShouldBeNil)
			budget := 24 * time.Hour
			viper.Set("cognitive.tick_budget", budget)
			c.So(wired.Crypto.Run(), ShouldBeNil)
			synctest.Wait()
			capacity := viper.GetInt("signals.feed_timeline_capacity")
			finished := make(chan error, 1)
			step := tests.MarketStep{
				Advance: time.Second,
				Actions: []tests.MarketAction{
					{Kind: tests.MarketTrade, Symbol: market.Symbols[0], Side: "buy", Qty: 1},
					{Kind: tests.MarketRefill, Symbol: market.Symbols[0], Side: "sell", Qty: 1},
				},
			}

			go func() {
				for range capacity + 1 {
					if err := market.Apply(step, func() error { return nil }); err != nil {
						finished <- err
						return
					}
				}

				finished <- nil
			}()

			for range 4 {
				time.Sleep(budget)
				synctest.Wait()
			}

			c.So(<-finished, ShouldBeNil)
			c.So(market.Apply(step, func() error { return nil }), ShouldBeNil)
			thesis, err := wired.Crypto.Tick()
			c.So(err, ShouldBeNil)
			c.So(thesis.Measurements, ShouldHaveLength, 136)
			c.So(thesis.Tick, ShouldEqual, int64(21))
			c.So(thesis.At, ShouldResemble, market.Now())
			c.So(thesis.Incomplete(), ShouldBeFalse)
			waitingAt := time.Now()

			c.So(wired.Crypto.Close(), ShouldBeNil)
			c.So(time.Now(), ShouldResemble, waitingAt)
		})
	})

	Convey("Given signal outputs close before runtime teardown", t, func(c C) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			market := tests.NewMarket(ctx, 1)
			wired, err := stack.NewBooter(ctx).Test(market)
			c.So(err, ShouldBeNil)
			defer market.Close()

			cancel()
			synctest.Wait()
			thesis, err := wired.Crypto.Tick()
			c.So(thesis, ShouldNotBeNil)
			c.So(err, ShouldBeNil)
			c.So(wired.Close(), ShouldBeNil)
		})
	})

	Convey("Given the production runtime owns every simulated market cut", t, func(c C) {
		synctest.Test(t, func(t *testing.T) {
			market := tests.NewMarket(t.Context(), 1)
			wired, err := stack.NewBooter(t.Context()).Test(market)
			c.So(err, ShouldBeNil)
			defer market.Close()
			defer func() { c.So(wired.Close(), ShouldBeNil) }()

			budget := 10 * time.Millisecond
			viper.Set("cognitive.tick_budget", budget)
			c.So(wired.Crypto.Run(), ShouldBeNil)
			synctest.Wait()
			cuts := 0
			consume := func() error {
				time.Sleep(budget)
				synctest.Wait()
				cuts++
				return market.Paper.Drain()
			}

			c.So(market.Warmup(consume), ShouldBeNil)
			c.So(cuts, ShouldEqual, 16)
			c.So(wired.Desk.OpenPositions(), ShouldEqual, 0)

			for range 3 {
				c.So(market.Transition(tests.MarketStateFastPump, consume), ShouldBeNil)
			}

			c.So(cuts, ShouldEqual, 52)
			c.So(wired.Desk.OpenPositions(), ShouldEqual, 1)
			symbol := market.Symbols[0]
			holding, err := wired.Balance.Holding(symbol)
			c.So(err, ShouldBeNil)
			c.So(holding.Symbol, ShouldEqual, symbol)
			c.So(holding.Status, ShouldEqual, types.OPEN)
			position, found := wired.Desk.Position(symbol)
			c.So(found, ShouldBeTrue)
			c.So(position.Status(), ShouldEqual, types.OPEN)
			pair, err := wired.Instrument.Pair(symbol)
			c.So(err, ShouldBeNil)
			feeFraction, err := wired.Price.Fraction(symbol)
			c.So(err, ShouldBeNil)
			var exitCost, exitFee, exitPrice, exitQuantity *decimal.Decimal
			market.Paper.On("executions", func(payload []byte) {
				for _, execution := range kraken.NewExecution(payload).Data {
					if execution.Symbol != symbol || execution.Side != "sell" ||
						execution.ExecType != "trade" {
						continue
					}

					exitCost = execution.Cost.Copy()
					exitFee = execution.FeeUsdEquiv.Copy()
					exitPrice = execution.LastPrice.Copy()
					exitQuantity = execution.LastQty.Copy()
				}
			})

			c.So(market.Paper.EnablePaper(mockapi.PaperOptions{
				Quote: func(symbol string) (float64, float64, float64, float64, bool) {
					ticker, quoteErr := wired.Price.Get(symbol)

					if quoteErr != nil {
						return 0, 0, 0, 0, false
					}

					return ticker.Bid.Float64(), ticker.BidQty,
						ticker.Ask.Float64(), ticker.AskQty, true
				},
				Now: market.Now,
				Balances: map[string]float64{
					"USD":     0,
					pair.Base: holding.Qty.Float64(),
				},
				MakerFee: feeFraction.Float64(),
				TakerFee: feeFraction.Float64(),
			}), ShouldBeNil)
			c.So(wired.API.SubscribeBalance(), ShouldBeNil)
			quoteCash, err := wired.Balance.AssetAvailable("USD")
			c.So(err, ShouldBeNil)
			c.So(quoteCash.Cmp(decimal.NewFromInt64(0)), ShouldEqual, 0)
			baseCash, err := wired.Balance.AssetAvailable(pair.Base)
			c.So(err, ShouldBeNil)
			c.So(baseCash.Cmp(holding.Qty), ShouldEqual, 0)

			var exitBid *decimal.Decimal
			exitStart := cuts
			c.So(market.Transition(tests.MarketStateFastDump, func() error {
				time.Sleep(budget)
				synctest.Wait()
				cuts++

				if position.Status() == types.PENDING && exitBid == nil {
					ticker, quoteErr := wired.Price.Get(symbol)

					if quoteErr != nil {
						return quoteErr
					}

					exitBid = ticker.Bid.Copy()
				}

				return market.Paper.Drain()
			}), ShouldBeNil)
			c.So(cuts-exitStart, ShouldEqual, 12)
			c.So(exitCost, ShouldNotBeNil)
			c.So(exitFee, ShouldNotBeNil)
			c.So(exitPrice.Cmp(exitBid), ShouldEqual, 0)
			c.So(exitQuantity.Cmp(holding.Qty), ShouldEqual, 0)
			c.So(wired.Desk.OpenPositions(), ShouldEqual, 0)
			_, err = wired.Balance.Holding(symbol)
			c.So(err, ShouldNotBeNil)
			baseCash, err = wired.Balance.AssetAvailable(pair.Base)
			c.So(err, ShouldBeNil)
			c.So(baseCash.Cmp(decimal.NewFromInt64(0)), ShouldEqual, 0)
			quoteCash, err = wired.Balance.AssetAvailable("USD")
			c.So(err, ShouldBeNil)
			c.So(quoteCash.Cmp(exitCost.Copy().Sub(exitFee)), ShouldEqual, 0)
			c.So(position.Status(), ShouldEqual, types.CLOSED)
			c.So(wired.Crypto.Status(), ShouldEqual, types.READY)
		})
	})
}
