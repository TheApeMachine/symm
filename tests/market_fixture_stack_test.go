//go:build !race

package tests

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/transport"
	tes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

func runAutoFillStackTest(t *testing.T, symbols []*tes.Symbol) {
	Convey("Given an executable production-stack position lifecycle", t,
		WithOrders(t, symbols, cmd.Boot, func(market *Market, system *cmd.System) {
			names := make([]string, len(system.Signals))

			for index, signal := range system.Signals {
				names[index] = signal.Name()
			}

			So(names, ShouldResemble, []string{
				"correlation", "cvd", "depthflow", "exhaustion", "hawkes",
				"leadlag", "liquidity", "pumpdump", "sentiment", "toxicity",
			})
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
	symbols := []*tes.Symbol{
		tes.NewSymbol("SIM1/USD", 64_000, 42),
	}

	Convey("Given the full system driven only by simulated venue data", t,
		WithOrders(t, symbols, cmd.Boot, func(market *Market, system *cmd.System) {
			market.WithAutoFill()

			Convey("When a pump continues before reversing", func() {
				So(market.Transition("SIM1/USD", tes.FastPump), ShouldBeNil)
				So(market.Express("SIM1/USD"), ShouldBeNil)
				So(system.Desk.Holding("SIM1/USD"), ShouldBeGreaterThan, 0)

				So(market.Transition("SIM1/USD", tes.FastPump), ShouldBeNil)
				So(market.Express("SIM1/USD"), ShouldBeNil)
				So(market.Transition("SIM1/USD", tes.FastDump), ShouldBeNil)
				So(market.Flatten("SIM1/USD"), ShouldBeNil)

				Convey("Then the system should have entered and exited an actual lot", func() {
					So(system.Desk.Holding("SIM1/USD"), ShouldEqual, 0)
					closed := 0

					symbolState := system.Thesis.Symbol("SIM1/USD")

					for stored := range symbolState.Positions.Drain(symbolState.PositionConsumers[0], func(any) bool {
						return true
					}) {
						position, ok := stored.(*broker.Position)

						if !ok {
							continue
						}

						if position.Holding == nil ||
							position.Holding.Symbol != "SIM1/USD" ||
							position.Holding.Status != types.CLOSED {
							continue
						}

						closed++
						So(position.Holding.EntryAt, ShouldNotBeNil)
						So(position.Holding.EntryPrice, ShouldNotBeNil)
						So(position.Holding.ExitAt, ShouldNotBeNil)
						So(position.Holding.ExitPrice, ShouldNotBeNil)
						So(position.Holding.PnL, ShouldNotBeNil)
						So(position.Holding.PnL.Sign(), ShouldEqual, 1)
						So(position.Holding.ReturnPct, ShouldBeGreaterThan, 0.0)
					}

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
	symbol := tes.NewSymbol("IDOS/USD", 0.00455, 13)
	symbol.PriceIncrement = 0.00001
	symbol.PricePrecision = 5
	symbol.QuantityPrecision = 5
	symbol.TakerFeePercent = 0.4
	symbol.MakerFeePercent = 0.23
	symbol.BookDepthLevels = 10
	config := tes.NewScenarioConfig([]*tes.Symbol{symbol})
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

				symbolState := system.Thesis.Symbol("IDOS/USD")
				resonanceReady := make(chan struct{}, 1)
				resonanceConsumer := transport.NewConsumer[any]("replay-resonance", func() {
					select {
					case resonanceReady <- struct{}{}:
					default:
					}
				})
				positionReady := make(chan struct{}, 1)
				positionConsumer := transport.NewConsumer[any]("replay-position", func() {
					select {
					case positionReady <- struct{}{}:
					default:
					}
				})
				symbolState.Resonance.Register(resonanceConsumer)
				defer symbolState.Resonance.Unregister(resonanceConsumer)
				symbolState.Positions.Register(positionConsumer)
				defer symbolState.Positions.Unregister(positionConsumer)
				So(market.Replay(capture), ShouldBeNil)
				So(market.Public.Fence(), ShouldBeTrue)
				So(market.Private.Fence(), ShouldBeTrue)
				So(market.Level3.Fence(), ShouldBeTrue)
				So(system.Thesis.WaitForQuiescence(t.Context()), ShouldBeNil)

				var coder *learning.ResonanceManifold
				resonanceCtx, cancelResonance := context.WithTimeout(
					market.ctx,
					market.Config.BookApplyTimeout,
				)
				defer cancelResonance()

			resonanceLoop:
				for coder == nil {
					select {
					case <-resonanceCtx.Done():
						break resonanceLoop
					case <-resonanceReady:
					}

					for stored := range symbolState.Resonance.Drain(resonanceConsumer, nil) {
						if candidate, valid := stored.(*learning.ResonanceManifold); valid && candidate != nil {
							coder = candidate
						}
					}
				}

				So(coder, ShouldNotBeNil)
				forecast, forecastErr := coder.RolloutTaskForecast(1)
				_, skillReady := coder.TaskSkill()
				var completed *broker.Position
				var active *broker.Position
				positionCtx, cancel := context.WithTimeout(
					market.ctx,
					market.Config.BookApplyTimeout,
				)
				defer cancel()

			positionLoop:
				for completed == nil {
					select {
					case <-positionCtx.Done():
						break positionLoop
					case <-positionReady:
					}

					for stored := range symbolState.Positions.Drain(positionConsumer, nil) {
						position, valid := stored.(*broker.Position)

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
					}
				}

				for stored := range symbolState.Positions.Drain(symbolState.PositionConsumers[0], func(any) bool {
					return true
				}) {
					position, valid := stored.(*broker.Position)

					if valid && position.Holding != nil &&
						position.Holding.Status != types.CLOSED {
						active = position
					}
				}

				Convey("It should carry each structurally admitted lifecycle through the streamed graph sequence", func() {
					So(forecastErr, ShouldBeNil)
					So(forecast, ShouldNotBeEmpty)
					So(skillReady, ShouldBeTrue)
					So(active, ShouldNotBeNil)
					So(active.Decision.GraphScore, ShouldBeGreaterThanOrEqualTo,
						active.Decision.AdmissionGraphThreshold)
					So(active.Holding.Status, ShouldNotEqual, types.CLOSED)
					So(market.Validate(), ShouldBeNil)

					if completed != nil {
						So(active.Decision.ID, ShouldNotEqual, completed.Decision.ID)
						So(active.Holding.EntryAt, ShouldNotBeNil)

						if active.Holding.EntryAt != nil && completed.Holding.ExitAt != nil {
							So(active.Holding.EntryAt.After(*completed.Holding.ExitAt), ShouldBeTrue)
						}
					}
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

	config := tes.NewScenarioConfig(symbols)
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
				rows := captureCrossSection(system, symbolNames)
				profitable := make([]*broker.Position, 0)

				for _, row := range rows {
					t.Log(row.String())
					profitable = append(profitable, row.profitable...)
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

func TestMarketCaptureHoldouts(t *testing.T) {
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
	captured, err := CaptureSymbols(pairs, metadataTickers, 10)

	if err != nil {
		t.Fatal(err)
	}

	holdouts := []string{"AKE/USD", "IDOS/USD", "CRV/USD"}
	symbols := selectCaptureSymbols(captured, holdouts...)

	if len(symbols) != len(holdouts) {
		t.Fatalf("holdout symbols missing from capture: have %d want %d",
			len(symbols), len(holdouts))
	}

	config := tes.NewScenarioConfig(symbols)
	config.InitialBalances = map[string]float64{"USD": 200}

	Convey("Given AKE, IDOS, and CRV from the same captured session", t,
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

				So(market.ReplayCapture(holdouts, ticker, trades, level3), ShouldBeNil)
				rows := captureCrossSection(system, holdouts)

				for _, row := range rows {
					t.Log(row.String())
				}

				Convey("It should report a valid session without requiring the IDOS melt-up", func() {
					So(market.Validate(), ShouldBeNil)
					So(rows, ShouldHaveLength, 3)
					t.Logf(
						"holdout net PnL: %.9f submitted=%d filled=%d",
						market.Report().Economics.NetPnL,
						market.Report().Mechanics.Submitted,
						market.Report().Mechanics.Filled,
					)
				})
			}),
		),
	)
}

type captureSymbolRow struct {
	symbol     string
	action     types.Action
	reason     string
	utility    float64
	graph      float64
	sources    int
	horizon    int
	forecast   float64
	ready      bool
	skill      float64
	skillReady bool
	open       int
	closed     int
	closedPnL  float64
	profitable []*broker.Position
}

func (row captureSymbolRow) String() string {
	return fmt.Sprintf(
		"capture %s action=%s reason=%q utility=%.6f graph=%.6f sources=%d horizon=%d forecast=%.6f ready=%t skill=%.4f/%t open=%d closed=%d closedPnL=%.6f",
		row.symbol,
		row.action,
		row.reason,
		row.utility,
		row.graph,
		row.sources,
		row.horizon,
		row.forecast,
		row.ready,
		row.skill,
		row.skillReady,
		row.open,
		row.closed,
		row.closedPnL,
	)
}

func captureCrossSection(system *cmd.System, names []string) []captureSymbolRow {
	rows := make([]captureSymbolRow, 0, len(names))

	for _, name := range names {
		row := captureSymbolRow{symbol: name, action: types.ActionNothing}
		symbolState := system.Thesis.Symbol(name)

		var decision types.Decision

		for candidate := range symbolState.Decisions.Drain(symbolState.DecisionConsumers[0], func(types.Decision) bool {
			return true
		}) {
			decision = candidate
		}

		if decision.Symbol != "" {
			row.action = decision.Action
			row.reason = decision.Reason
			row.utility = decision.Utility
			row.graph = decision.GraphScore
			row.sources = len(decision.PerspectiveSources)
			row.horizon = decision.ForecastHorizon
		}

		var coder *learning.ResonanceManifold

		for stored := range symbolState.Resonance.Drain(
			symbolState.ResonanceConsumers[types.ResonanceConsumerAudit],
			func(any) bool {
				return true
			},
		) {
			if candidate, valid := stored.(*learning.ResonanceManifold); valid && candidate != nil {
				coder = candidate
			}
		}

		if coder != nil {
			forecast, _ := coder.RolloutTaskForecast(1)
			row.skill, row.skillReady = coder.TaskSkill()

			if len(forecast) > 0 {
				row.forecast = forecast[0].Value
				row.ready = forecast[0].Ready
			}
		}

		for stuck := range symbolState.Positions.Drain(symbolState.PositionConsumers[0], func(any) bool {
			return true
		}) {
			position, valid := stuck.(*broker.Position)

			if !valid || position == nil || position.Holding == nil {
				continue
			}

			if position.Holding.Status == types.CLOSED {
				row.closed++

				if position.Holding.PnL != nil {
					row.closedPnL += position.Holding.PnL.Float64()

					if position.Holding.PnL.Sign() > 0 {
						row.profitable = append(row.profitable, position)
					}
				}

				continue
			}

			row.open++
		}

		rows = append(rows, row)
	}

	return rows
}

func selectCaptureSymbols(
	symbols []*tes.Symbol,
	names ...string,
) []*tes.Symbol {
	wanted := make(map[string]struct{}, len(names))

	for _, name := range names {
		wanted[name] = struct{}{}
	}

	selected := make([]*tes.Symbol, 0, len(names))

	for _, symbol := range symbols {
		if _, found := wanted[symbol.Pair]; !found {
			continue
		}

		selected = append(selected, symbol)
	}

	return selected
}
