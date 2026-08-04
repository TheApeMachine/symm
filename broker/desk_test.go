package broker_test

import (
	"bytes"
	"errors"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/phuslu/log"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/tests"
	executionfixture "github.com/theapemachine/symm/tests/fixtures/execution"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

/*
entryDecision builds a sized entry carrying the stop geometry it was sized
under.

The plan is not decoration. The desk refuses an entry without one, because the
quantity travelling with a decision was solved against a particular risk
distance and attaching some other distance after the fact breaks the coupling
that makes a wide stop affordable. A bare decision here would only be testing a
path production no longer has.
*/
func entryDecision(market *tests.Market, symbol string) types.Decision {
	decision := types.Decision{
		ID:               uuid.NewString(),
		Action:           types.ActionEnter,
		Symbol:           symbol,
		ProposedQuantity: decimal.NewFromFloat64(0.25),
		ProposedNotional: decimal.NewFromInt64(25),
	}

	if pair, err := market.Desk.Instrument().Pair(symbol); err == nil {
		decision.Risk = market.Desk.Price().RiskPlan(pair)
	}

	return decision
}

func TestDeskExecute(t *testing.T) {
	Convey("Given a production-wired simulated market", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
			testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
			testtypes.NewSymbol("SIM3/USD", 100.0, 90210),
		}

		Convey("Execute should submit entries but reject strategy exits", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			market.Tick()
			decision := entryDecision(market, symbols[0].Pair)

			So(market.Desk.Execute([]types.Decision{decision}), ShouldBeNil)
			So(market.Desk.OpenPositions(), ShouldEqual, 1)

			positions := slices.Collect(market.Desk.Positions())
			So(positions, ShouldHaveLength, 1)
			So(positions[0].ID, ShouldEqual, decision.ID)
			So(positions[0].EntryOrder.ClOrdId, ShouldEqual, decision.ID)
			So(positions[0].Status, ShouldEqual, types.PENDING)

			fill := executionfixture.BuyFill()
			fill.ClientOrderID = decision.ID
			fill.Symbol = decision.Symbol
			fill.AvgPrice = fillPrice(market, decision.Symbol)
			fill.CumQty = decision.ProposedQuantity.String()
			market.Private.Publish("executions", executionfixture.Frame(fill))
			market.Tick()

			err := market.Desk.Execute([]types.Decision{{
				ID:     uuid.NewString(),
				Action: types.ActionExit,
				Symbol: decision.Symbol,
			}})

			positions = slices.Collect(market.Desk.Positions())
			So(positions, ShouldHaveLength, 1)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "only a triggered stoploss may submit a sell")
			So(positions[0].Status, ShouldEqual, types.OPEN)
			So(positions[0].ExitOrderResult, ShouldBeNil)
		}))

		Convey("A repeated enter should reject the new position", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			market.Tick()
			decision := entryDecision(market, symbols[0].Pair)
			So(market.Desk.Execute([]types.Decision{decision}), ShouldBeNil)
			openPositions := market.Desk.OpenPositions()

			duplicate := entryDecision(market, decision.Symbol)
			err := market.Desk.Execute([]types.Decision{duplicate})

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "symbol already has an active position")
			So(market.Desk.OpenPositions(), ShouldEqual, openPositions)
			So(slices.Collect(market.Desk.Positions()), ShouldHaveLength, openPositions)
		}))

		Convey("A strategy exit should be rejected without inspecting inventory", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			openPositions := market.Desk.OpenPositions()
			err := market.Desk.Execute([]types.Decision{{
				ID:     uuid.NewString(),
				Action: types.ActionExit,
				Symbol: symbols[0].Pair,
			}})

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "only a triggered stoploss may submit a sell")
			So(market.Desk.OpenPositions(), ShouldEqual, openPositions)
			So(slices.Collect(market.Desk.Positions()), ShouldBeEmpty)
		}))

		Convey("An AddOrder failure should release the attempted position", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			market.Tick()
			market.Private.FailAddOrder(errors.New("venue unavailable"))
			openPositions := market.Desk.OpenPositions()
			err := market.Desk.Execute([]types.Decision{entryDecision(market, symbols[0].Pair)})

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "failed to place market order")
			So(market.Desk.OpenPositions(), ShouldEqual, openPositions)
			So(slices.Collect(market.Desk.Positions()), ShouldBeEmpty)
		}))

		Convey("Concurrent entries should not exceed normal capacity", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			market.Tick()
			executionErrors := make(chan error, len(symbols))
			wait := sync.WaitGroup{}

			for _, symbol := range symbols {
				wait.Add(1)

				go func() {
					defer wait.Done()
					executionErrors <- market.Desk.Execute([]types.Decision{entryDecision(market, symbol.Pair)})
				}()
			}

			wait.Wait()
			close(executionErrors)
			rejections := 0

			for err := range executionErrors {
				if err != nil {
					rejections++
				}
			}

			So(rejections, ShouldEqual, 1)
			So(market.Desk.OpenPositions(), ShouldEqual, market.Desk.MaxPositions())
			So(slices.Collect(market.Desk.Positions()), ShouldHaveLength, market.Desk.MaxPositions())
		}))
	})
}

func TestDeskRecover(t *testing.T) {
	Convey("Given wallet inventory and its acquisition history at boot", t, func() {
		tradingModel := viper.GetString("trading.model")
		apiKey, hadAPIKey := os.LookupEnv("KRAKEN_API_KEY")
		apiSecret, hadAPISecret := os.LookupEnv("KRAKEN_API_SECRET")

		viper.Set("trading.model", "real")
		_ = os.Setenv("KRAKEN_API_KEY", "fixture-key")
		_ = os.Setenv("KRAKEN_API_SECRET", "Zml4dHVyZS1zZWNyZXQ=")

		defer func() {
			viper.Set("trading.model", tradingModel)

			if hadAPIKey {
				_ = os.Setenv("KRAKEN_API_KEY", apiKey)
			} else {
				_ = os.Unsetenv("KRAKEN_API_KEY")
			}

			if hadAPISecret {
				_ = os.Setenv("KRAKEN_API_SECRET", apiSecret)
			} else {
				_ = os.Unsetenv("KRAKEN_API_SECRET")
			}
		}()

		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
			testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
		}
		entryAt := time.Now().UTC().Add(-time.Hour)
		market := tests.NewMarketWithAccount(
			t.Context(),
			symbols,
			map[string]string{"USD": "150", "SIM2": "1.5"},
			map[string]spot.Trade{
				"entry": {
					Pair:   "sim2usd",
					Time:   decimal.NewFromFloat64(float64(entryAt.UnixNano()) / 1e9),
					Type:   "buy",
					Cost:   decimal.NewFromInt64(150),
					Fee:    decimal.NewFromFloat64(0.39),
					Volume: decimal.NewFromFloat64(1.5),
				},
			},
		)
		logs := &bytes.Buffer{}
		originalWriter := log.DefaultLogger.Writer
		log.DefaultLogger.Writer = log.IOWriter{Writer: logs}

		defer func() {
			market.Close()
			log.DefaultLogger.Writer = originalWriter
		}()

		Convey("The desk should adopt the known lot immediately and let the first ticker take over its mark", func() {
			positions := slices.Collect(market.Desk.Positions())

			So(positions, ShouldHaveLength, 1)
			So(market.Desk.OpenPositions(), ShouldEqual, 1)
			So(positions[0].ID, ShouldEqual, "recovered:SIM2/USD")
			So(positions[0].Status, ShouldEqual, types.OPEN)
			So(positions[0].Holding.Symbol, ShouldEqual, "SIM2/USD")
			So(positions[0].Holding.Qty.Cmp(decimal.NewFromFloat64(1.5)), ShouldEqual, 0)
			So(positions[0].Holding.SellableQty.Cmp(decimal.NewFromFloat64(1.5)), ShouldEqual, 0)
			So(positions[0].Holding.EntryPrice.Float64(), ShouldEqual, 100.0)
			So(positions[0].Holding.EntryFee.Float64(), ShouldAlmostEqual, 0.39, 1e-8)
			So(positions[0].Holding.EntryAt, ShouldNotBeNil)
			So(positions[0].Holding.Mark, ShouldNotBeNil)
			So(positions[0].Holding.Mark.Cmp(positions[0].Holding.EntryPrice), ShouldEqual, 0)
			So(positions[0].Holding.Stoploss, ShouldNotBeNil)
			So(positions[0].Holding.Stoploss.Entry.Cmp(positions[0].Holding.EntryPrice), ShouldEqual, 0)
			So(positions[0].Holding.Stoploss.Mark.Cmp(positions[0].Holding.EntryPrice), ShouldEqual, 0)

			/*
				Recovery runs before anything has priced the symbol, so the lot
				is adopted without a floor rather than with one invented from a
				book nobody has seen. The alternative is a boundary drawn from
				tick granularity alone, which on a liquid pair lands a fraction
				of a percent under the entry and stops the recovered lot out on
				the first tick that prices it.
			*/
			So(positions[0].Holding.Stoploss.Floor, ShouldBeNil)

			market.Tick()
			positions = slices.Collect(market.Desk.Positions())

			So(market.Desk.Price().Tick("sim2usd"), ShouldNotBeNil)
			So(positions[0].Holding.EntryPrice.Float64(), ShouldEqual, 100.0)
			So(positions[0].Holding.Mark.Cmp(positions[0].Holding.EntryPrice), ShouldNotEqual, 0)
			So(positions[0].Holding.Stoploss.Entry.Cmp(positions[0].Holding.EntryPrice), ShouldEqual, 0)
			So(positions[0].Holding.Stoploss.Mark.Cmp(positions[0].Holding.Mark), ShouldEqual, 0)

			// The first ticker is what makes the lot defensible: geometry is
			// adopted from the book it finally has.
			So(positions[0].Holding.Stoploss.Plan.Present, ShouldBeTrue)
			So(positions[0].Holding.Stoploss.Floor, ShouldNotBeNil)
			So(positions[0].Holding.Stoploss.HardFloor.Cmp(positions[0].Holding.EntryPrice), ShouldEqual, -1)
			So(bytes.Contains(logs.Bytes(), []byte("ticker not found")), ShouldBeFalse)
		})
	})
}
