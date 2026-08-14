//go:build !race

package tests

import (
	"os"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/kraken"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

func runAutoFillStackTest(t *testing.T, symbols []*testtypes.Symbol) {
	Convey("Given an executable production-stack position lifecycle", t,
		WithOrders(t, symbols, cmd.Boot, func(market *Market, _ *cmd.System) {
			market.WithAutoFill()
			market.Tick()
			_, private := market.Feeds()
			executions := make(chan *kraken.Execution, 1)
			handler := market.Private.Client().OnReceived.Recurring(
				func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
					execution := kraken.NewExecution(event.Data.Bytes())

					if execution.Channel == "executions" {
						executions <- execution
					}
				},
			)
			defer market.Private.Client().OnReceived.Deregister(handler)
			result, err := private.AddOrder(&spot.AddOrderRequest{
				ClOrdId: "entry-1", OrderType: "market", Type: "buy",
				Volume: "0.25", Pair: symbols[0].Pair,
			})
			So(err, ShouldBeNil)
			So(result.ID, ShouldHaveLength, 1)
			market.Tick()
			var fill *kraken.Execution

			select {
			case fill = <-executions:
			default:
			}

			So(fill, ShouldNotBeNil)
			So(fill.Data, ShouldHaveLength, 1)
			So(fill.Data[0].OrderID, ShouldEqual, result.ID[0])
			So(fill.Data[0].ClientOrderID, ShouldEqual, "entry-1")
			So(fill.Data[0].Symbol, ShouldEqual, symbols[0].Pair)
			So(fill.Data[0].Side, ShouldEqual, "buy")
			So(fill.Data[0].AvgPrice.Float64(),
				ShouldEqual, market.latest[symbols[0].Pair].Ask)
			expectedFee := fill.Data[0].Cost.Float64() *
				simulatedTakerFeePercent / percentDenominator
			So(fill.Data[0].FeeUsdEquiv.Float64(),
				ShouldAlmostEqual, expectedFee, 1e-12)
		}),
	)
}

func TestMarketStackEntryAndExit(t *testing.T) {
	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 64_000, 42),
	}

	Convey("Given the full system driven only by simulated venue data", t,
		WithOrders(t, symbols, cmd.Boot, func(market *Market, system *cmd.System) {
			market.WithAutoFill()

			Convey("When a pump continues before reversing", func() {
				So(market.Transition("SIM1/USD", testtypes.FastPump), ShouldBeNil)
				So(market.Express("SIM1/USD"), ShouldBeNil)
				So(system.Desk.Holding("SIM1/USD"), ShouldBeGreaterThan, 0)

				So(market.Transition("SIM1/USD", testtypes.FastPump), ShouldBeNil)
				So(market.Express("SIM1/USD"), ShouldBeNil)
				So(market.Transition("SIM1/USD", testtypes.FastDump), ShouldBeNil)
				So(market.Flatten("SIM1/USD"), ShouldBeNil)

				Convey("Then the system should have entered and exited an actual lot", func() {
					So(system.Desk.Holding("SIM1/USD"), ShouldEqual, 0)
					closed := 0

					system.Thesis.Symbol("SIM1/USD").Positions.Range(func(_, value any) bool {
						position, ok := value.(*broker.Position)

						if !ok {
							return true
						}

						if position.Holding == nil ||
							position.Holding.Symbol != "SIM1/USD" ||
							position.Holding.Status != types.CLOSED {
							return true
						}

						closed++
						So(position.Holding.EntryAt, ShouldNotBeNil)
						So(position.Holding.EntryPrice, ShouldNotBeNil)
						So(position.Holding.ExitAt, ShouldNotBeNil)
						So(position.Holding.ExitPrice, ShouldNotBeNil)
						So(position.Holding.PnL, ShouldNotBeNil)
						So(position.Holding.PnL.Sign(), ShouldEqual, 1)
						So(position.Holding.ReturnPct, ShouldBeGreaterThan, 0.0)
						return true
					})

					So(closed, ShouldBeGreaterThan, 0)
				})
			})
		}),
	)
}

func TestMarketReplayEntryAndExit(t *testing.T) {
	previousDepth, depthWasSet := viper.GetInt("market.l3_depth"),
		viper.IsSet("market.l3_depth")
	viper.Set("market.l3_depth", 10)
	defer func() {
		if depthWasSet {
			viper.Set("market.l3_depth", previousDepth)
			return
		}

		viper.Set("market.l3_depth", nil)
	}()
	symbol := testtypes.NewSymbol("IDOS/USD", 0.00455, 13)
	symbol.PriceIncrement = 0.00001
	symbol.PricePrecision = 5
	symbol.QuantityPrecision = 5
	symbol.TakerFeePercent = 0.4
	symbol.MakerFeePercent = 0.23
	symbol.BookDepthLevels = 10
	config := testtypes.NewScenarioConfig([]*testtypes.Symbol{symbol})
	config.InitialBalances = map[string]float64{"USD": 200}

	Convey("Given an IDOS/USD slice with a resolved adaptive forecast horizon", t,
		WithFixtureOrderScenario(t, config,
			drive(t, cmd.Boot, func(market *Market, system *cmd.System) {
				execution := market.Config.Execution
				execution.DepthLevels = 10
				market.WithAutoFill(execution)
				capture, err := os.Open(
					"/Users/theapemachine/.symm/data/backtests/kraken/" +
						"2026-08-13-live-exact-v2/slices/IDOSUSD.jsonl",
				)
				So(err, ShouldBeNil)
				defer capture.Close()

				So(market.Replay(capture), ShouldBeNil)
				symbolState := system.Thesis.Symbol("IDOS/USD")
				stored, found := symbolState.Resonance.Load("IDOS/USD")
				So(found, ShouldBeTrue)
				coder := stored.(*learning.ResonanceManifold)
				forecast, forecastErr := coder.RolloutTaskForecast(1)
				skill, skillReady := coder.TaskSkill()
				var completed *broker.Position
				var active *broker.Position
				symbolState.Positions.Range(func(_, value any) bool {
					position, valid := value.(*broker.Position)

					if valid && position.Holding != nil {
						t.Logf(
							"exact slice position: status=%s utility=%g graph=%g sources=%#v pnl=%v return=%g",
							position.Holding.Status,
							position.Decision.Utility,
							position.Decision.GraphScore,
							position.Decision.PerspectiveSources,
							position.Holding.PnL,
							position.Holding.ReturnPct,
						)
					}

					if valid && position.Holding != nil &&
						position.Holding.Status == types.CLOSED {
						completed = position
					}

					if valid && position.Holding != nil &&
						position.Holding.Status != types.CLOSED {
						active = position
					}

					return true
				})

				Convey("It should enter only on calibrated positive net utility and exit profitably", func() {
					So(forecastErr, ShouldBeNil)
					So(forecast, ShouldNotBeEmpty)
					So(skillReady, ShouldBeTrue)
					So(completed, ShouldNotBeNil)
					So(completed.Decision.ForecastHorizon, ShouldBeGreaterThan, 0)
					So(completed.Decision.Utility, ShouldBeGreaterThan, 0)
					So(completed.Decision.GraphScore, ShouldBeGreaterThanOrEqualTo,
						completed.Decision.AdmissionGraphThreshold)
					So(completed.Holding.PnL.Sign(), ShouldEqual, 1)
					So(market.Report().Economics.NetPnL, ShouldBeGreaterThan, 0.0)
					So(market.Validate(), ShouldBeNil)

					if active != nil {
						So(active.Decision.ID, ShouldNotEqual, completed.Decision.ID)
						So(active.Holding.EntryAt, ShouldNotBeNil)

						if active.Holding.EntryAt != nil {
							So(active.Holding.EntryAt.After(*completed.Holding.ExitAt), ShouldBeTrue)
						}
					}

					t.Logf(
						"exact slice result: forecast=%#v horizon=%d skill=%g/%t net=%g pnl=%s",
						forecast,
						completed.Decision.ForecastHorizon,
						skill,
						skillReady,
						completed.Decision.Utility,
						completed.Holding.PnL,
					)
				})
			}),
		),
	)
}

func TestMarketCaptureEntryAndExit(t *testing.T) {
	previousDepth, depthWasSet := viper.GetInt("market.l3_depth"),
		viper.IsSet("market.l3_depth")
	viper.Set("market.l3_depth", 10)
	defer func() {
		if depthWasSet {
			viper.Set("market.l3_depth", previousDepth)
			return
		}

		viper.Set("market.l3_depth", nil)
	}()
	captureDirectory := "/Users/theapemachine/.symm/data/backtests/" +
		"kraken/2026-08-13-live-exact-v2/"
	pairs, err := os.Open(captureDirectory + "pairs.json")

	if err != nil {
		t.Fatal(err)
	}

	defer pairs.Close()
	metadataTickers, err := os.Open(captureDirectory + "ticker.jsonl")

	if err != nil {
		t.Fatal(err)
	}

	defer metadataTickers.Close()
	symbols, err := CaptureSymbols(pairs, metadataTickers, 10)

	if err != nil {
		t.Fatal(err)
	}

	config := testtypes.NewScenarioConfig(symbols)
	config.InitialBalances = map[string]float64{"USD": 200}
	symbolNames := make([]string, len(symbols))

	for index, symbol := range symbols {
		symbolNames[index] = symbol.Pair
	}

	Convey("Given the complete captured Kraken market", t,
		WithFixtureOrderScenario(t, config,
			drive(t, cmd.Boot, func(market *Market, system *cmd.System) {
				execution := market.Config.Execution
				execution.DepthLevels = 10
				market.WithAutoFill(execution)
				ticker, err := os.Open(captureDirectory + "ticker.jsonl")
				So(err, ShouldBeNil)
				defer ticker.Close()
				trades, err := os.Open(captureDirectory + "trade.jsonl")
				So(err, ShouldBeNil)
				defer trades.Close()
				level3, err := os.Open(captureDirectory + "level3.jsonl")
				So(err, ShouldBeNil)
				defer level3.Close()

				So(market.ReplayCapture(
					symbolNames,
					ticker,
					trades,
					level3,
				), ShouldBeNil)
				profitable := make([]*broker.Position, 0)

				for _, name := range symbolNames {
					symbolState := system.Thesis.Symbol(name)
					symbolState.Positions.Range(func(_, value any) bool {
						position, valid := value.(*broker.Position)

						if valid && position.Holding != nil &&
							position.Holding.Status == types.CLOSED &&
							position.Holding.PnL != nil &&
							position.Holding.PnL.Sign() > 0 {
							profitable = append(profitable, position)
						}

						return true
					})

					if stored, found := symbolState.Resonance.Load(name); found {
						coder := stored.(*learning.ResonanceManifold)
						forecast, _ := coder.RolloutTaskForecast(1)
						skill, skillReady := coder.TaskSkill()
						t.Logf(
							"capture calibration %s: forecast=%#v skill=%g/%t",
							name,
							forecast,
							skill,
							skillReady,
						)
					}
				}

				Convey("It should retain a profitable completed position", func() {
					So(profitable, ShouldNotBeEmpty)
					So(market.Report().Economics.NetPnL, ShouldBeGreaterThan, 0.0)
					So(market.Validate(), ShouldBeNil)

					for _, position := range profitable {
						t.Logf(
							"full capture result %s: entry=%s@%s exit=%s@%s pnl=%s return=%.6f%%",
							position.Holding.Symbol,
							position.Holding.EntryAt,
							position.Holding.EntryPrice,
							position.Holding.ExitAt,
							position.Holding.ExitPrice,
							position.Holding.PnL,
							position.Holding.ReturnPct,
						)
					}

					t.Logf("full capture net PnL: %.9f", market.Report().Economics.NetPnL)
				})
			}),
		),
	)
}
