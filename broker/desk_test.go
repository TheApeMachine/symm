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

func entryDecision(symbol string) types.Decision {
	return types.Decision{
		ID:               uuid.NewString(),
		Action:           types.ActionEnter,
		Symbol:           symbol,
		ProposedQuantity: decimal.NewFromFloat64(0.25),
		ProposedNotional: decimal.NewFromInt64(25),
	}
}

func TestDeskExecute(t *testing.T) {
	Convey("Given a production-wired simulated market", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
			testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
			testtypes.NewSymbol("SIM3/USD", 100.0, 90210),
		}

		Convey("Execute should submit and retain a position before a fill", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			market.Tick()
			decision := entryDecision(symbols[0].Pair)

			So(market.Desk.Execute([]types.Decision{decision}), ShouldBeNil)
			So(market.Desk.OpenPositions(), ShouldEqual, 1)

			positions := slices.Collect(market.Desk.Positions())
			So(positions, ShouldHaveLength, 1)
			So(positions[0].ID, ShouldEqual, decision.ID)
			So(positions[0].EntryOrder.ClOrdId, ShouldEqual, decision.ID)
			So(positions[0].EntryOrderID, ShouldNotBeBlank)
			So(positions[0].Status, ShouldEqual, types.PENDING)

			fill := executionfixture.BuyFill()
			fill.ClientOrderID = decision.ID
			fill.Symbol = decision.Symbol
			fill.CumQty = decision.ProposedQuantity.String()
			market.Private.Publish("executions", executionfixture.Frame(fill))
			market.Tick()

			exitID := uuid.NewString()
			So(market.Desk.Execute([]types.Decision{{
				ID:     exitID,
				Action: types.ActionExit,
				Symbol: decision.Symbol,
			}}), ShouldBeNil)

			positions = slices.Collect(market.Desk.Positions())
			So(positions, ShouldHaveLength, 1)
			So(positions[0].ExitOrder.ClOrdId, ShouldEqual, exitID)
		}))

		Convey("A repeated enter should reject the new position", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			market.Tick()
			decision := entryDecision(symbols[0].Pair)
			So(market.Desk.Execute([]types.Decision{decision}), ShouldBeNil)
			openPositions := market.Desk.OpenPositions()

			duplicate := entryDecision(decision.Symbol)
			err := market.Desk.Execute([]types.Decision{duplicate})

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "symbol already has an active position")
			So(market.Desk.OpenPositions(), ShouldEqual, openPositions)
			So(slices.Collect(market.Desk.Positions()), ShouldHaveLength, openPositions)
		}))

		Convey("An exit without an owned position should be rejected", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			openPositions := market.Desk.OpenPositions()
			err := market.Desk.Execute([]types.Decision{{
				ID:     uuid.NewString(),
				Action: types.ActionExit,
				Symbol: symbols[0].Pair,
			}})

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "active position not found for exit")
			So(market.Desk.OpenPositions(), ShouldEqual, openPositions)
			So(slices.Collect(market.Desk.Positions()), ShouldBeEmpty)
		}))

		Convey("An AddOrder failure should release the attempted position", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			market.Tick()
			market.Private.FailAddOrder(errors.New("venue unavailable"))
			openPositions := market.Desk.OpenPositions()
			err := market.Desk.Execute([]types.Decision{entryDecision(symbols[0].Pair)})

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
					executionErrors <- market.Desk.Execute([]types.Decision{entryDecision(symbol.Pair)})
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

		Convey("The desk should wait for a live mark before adopting the lot", func() {
			So(market.Desk.OpenPositions(), ShouldEqual, 0)

			market.Tick()
			positions := slices.Collect(market.Desk.Positions())

			So(positions, ShouldHaveLength, 1)
			So(positions[0].ID, ShouldEqual, "recovered:SIM2/USD")
			So(positions[0].Status, ShouldEqual, types.OPEN)
			So(positions[0].Holding.Symbol, ShouldEqual, "SIM2/USD")
			So(positions[0].Holding.Qty.String(), ShouldEqual, "1.5")
			So(positions[0].Holding.SellableQty.String(), ShouldEqual, "1.5")
			So(positions[0].Holding.EntryPrice.Float64(), ShouldEqual, 100.0)
			So(positions[0].Holding.EntryFee.Float64(), ShouldAlmostEqual, 0.39, 1e-8)
			So(positions[0].Holding.EntryAt, ShouldNotBeNil)
			So(positions[0].Holding.Mark, ShouldNotBeNil)
			So(positions[0].Holding.Stoploss, ShouldNotBeNil)
			So(market.Desk.Price().Tick("sim2usd"), ShouldNotBeNil)
			So(bytes.Contains(logs.Bytes(), []byte("ticker not found")), ShouldBeFalse)
		})
	})
}
