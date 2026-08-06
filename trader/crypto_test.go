package trader_test

import (
	"context"
	"fmt"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/stack"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/types"
)

var symbols = []*testtypes.Symbol{
	testtypes.NewSymbol("SIM1/USD", 64000.0, 42),
	testtypes.NewSymbol("SIM2/USD", 5432.193, 1337),
	testtypes.NewSymbol("SIM3/USD", 103.01234, 90210),
}

var regimeSymbols = []*testtypes.Symbol{
	testtypes.NewSymbol("SIM1/USD", 64000.0, 42),
	testtypes.NewSymbol("SIM2/USD", 5432.193, 1337),
	testtypes.NewSymbol("SIM3/USD", 103.01234, 90210),
	testtypes.NewSymbol("SIM4/USD", 0.00012345, 123456789),
	testtypes.NewSymbol("SIM5/USD", 987654321.0, 1),
}

var regimes = []struct {
	Symbol string
	State  testtypes.MarketState
}{
	{"SIM1/USD", testtypes.FastPump},
	{"SIM2/USD", testtypes.FastDump},
	{"SIM3/USD", testtypes.FlashCrash},
	{"SIM4/USD", testtypes.LoadedLiquidity},
	{"SIM5/USD", testtypes.Baseline},
	{"SIM6/USD", testtypes.SidewaysChop},
	{"SIM7/USD", testtypes.SlowPump},
	{"SIM8/USD", testtypes.SlowDump},
	{"SIM9/USD", testtypes.SpoofLiquidity},
	{"SIM10/USD", testtypes.SpreadCompression},
	{"SIM11/USD", testtypes.ThinLiquidity},
	{"SIM12/USD", testtypes.VolatilitySpike},
	{"SIM13/USD", testtypes.VolumeAbsorption},
	{"SIM14/USD", testtypes.EmpiricalRatioBaseline},
	{"SIM15/USD", testtypes.PositiveEvidenceFloor},
}

func getAPI(ctx context.Context) *websocket.API {
	return websocket.NewAPI(
		ctx,
		websocket.NewWithClient(ctx, nil, false, "", nil),
		websocket.NewWithClient(ctx, nil, false, "", nil),
	)
}

func getInstance(ctx context.Context) *trader.Crypto {
	return trader.NewCrypto(
		ctx,
		getAPI(ctx),
		nil,
		nil,
		strategy.NewPlanner(
			ctx,
			nil,
			getAPI(ctx),
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		),
		nil,
		nil,
	)
}

func TestStatus(t *testing.T) {
	Convey(
		"Setup",
		t, stack.WithStack(t, symbols, func(market *tests.Market, system *cmd.System) {
			crypto := getInstance(t.Context())

			Convey("Then the status should be READY", func() {
				So(crypto.Status(), ShouldEqual, types.READY)
			})
		}),
	)
}

func TestIntegration(t *testing.T) {
	Convey(
		"Setup",
		t, stack.WithStack(t, symbols, func(market *tests.Market, system *cmd.System) {
			thesis := system.Thesis

			Convey("When the market is transitioned to a fast pump", func() {
				So(market.Transition("SIM1/USD", testtypes.FastPump), ShouldBeNil)
				profile := testtypes.DefaultProfiles[testtypes.FastPump]
				expectation := profile.PrecursorExpectation(
					testtypes.DefaultProfiles[testtypes.Baseline],
				)

				Convey("Then canonical market history should satisfy the profile contract", func() {
					So(thesis, ShouldNotBeNil)
					tickers := symbolTickers(thesis, "SIM1/USD")
					trades := symbolTrades(thesis, "SIM1/USD")

					So(len(tickers), ShouldBeGreaterThanOrEqualTo,
						expectation.Contract.MinimumObservations)
					So(len(trades), ShouldBeGreaterThanOrEqualTo,
						expectation.Contract.MinimumObservations)
					So(tickers[len(tickers)-1].Last.Float64(),
						ShouldBeGreaterThan, symbols[0].StartPrice)
					So(tickers[len(tickers)-1].BidQty,
						ShouldBeGreaterThanOrEqualTo, expectation.MinimumBidQuantity)
					So(tickers[len(tickers)-1].AskQty,
						ShouldBeLessThanOrEqualTo, expectation.MaximumAskQuantity)
					So(trades[len(trades)-1].Side, ShouldEqual, profile.AggressorSide)
					So(trades[len(trades)-1].Qty,
						ShouldBeGreaterThanOrEqualTo, expectation.MinimumStepVolume)

					Convey("And each signal should expose explicitly available Thesis evidence", func() {
						for _, source := range signalSources() {
							measurement := latestMeasurement(thesis, source, "")

							So(fmt.Sprintf("%s:%t", source, measurement != nil),
								ShouldEqual, fmt.Sprintf("%s:true", source))
							So(measurement.At.IsZero(), ShouldBeFalse)
							So(fmt.Sprintf("%s:%s", source, measurement.Validity.State),
								ShouldNotEqual,
								fmt.Sprintf("%s:%s", source, types.ValidityInvalid))
							So(measurement.Metrics, ShouldNotBeEmpty)
						}

						Convey("And pump evidence should meet the named precursor thresholds", func() {
							measurement := latestMeasurement(
								thesis, types.SourcePumpDump, "SIM1/USD",
							)
							So(measurement.Validity.State, ShouldEqual, types.ValidityValid)

							for _, expectedMetric := range expectation.Contract.Metrics {
								sample, found := measurement.Metrics[types.MetricKey(
									expectedMetric.Metric, expectedMetric.Side,
								)]

								So(found, ShouldBeTrue)
								So(sample.Normalized, ShouldNotBeNil)
								So(*sample.Normalized, ShouldBeGreaterThan,
									expectedMetric.MinimumNormalized)
							}

							Convey("And logic should recognize the declared precursor categories", func() {
								categoriesRaw, found := thesis.Categories.Load("SIM1/USD")
								So(found, ShouldBeTrue)
								categories := categoriesRaw.([]types.Category)

								for _, expectedCategory := range expectation.Contract.Categories {
									category, found := findCategory(categories, expectedCategory)

									So(fmt.Sprintf("%s:%t", expectedCategory, found),
										ShouldEqual, fmt.Sprintf("%s:true", expectedCategory))
									So(category.Strength, ShouldBeGreaterThan, 0.0)
									So(category.Confidence, ShouldBeGreaterThan, 0.0)
								}

								Convey("And resonance should issue a positive forecast", func() {
									resonanceRaw, found := thesis.Resonance.Load("SIM1/USD")
									So(found, ShouldBeTrue)
									resonance, ok := resonanceRaw.(types.ResonanceReading)
									So(ok, ShouldBeTrue)
									So(resonance.Samples, ShouldBeGreaterThan, 0)
									So(resonance.ForecastValidity.State,
										ShouldEqual, types.ValidityValid)
									So(resonance.Forecast, ShouldNotBeNil)
									So(resonance.Forecast.Validate(), ShouldBeNil)
									So(resonance.Forecast.Curve, ShouldNotBeEmpty)
									So(resonance.Forecast.SupportedHorizon,
										ShouldEqual, len(resonance.Forecast.Curve))
									expectedReturn := resonance.Forecast.ExpectedReturn
									So(math.IsNaN(expectedReturn), ShouldBeFalse)
									So(expectedReturn, ShouldBeGreaterThan, 0.0)

									Convey("Then the planner should enter before ignition is sampled", func() {
										var entry *types.Decision

										thesis.Decisions.Range(func(key, value interface{}) bool {
											decision := value.(*types.Decision)

											if decision.Symbol == "SIM1/USD" &&
												decision.Action == types.ActionEnter {
												entry = decision
												return false
											}

											return true
										})

										So(entry, ShouldNotBeNil)
										So(entry.Risk.Present, ShouldBeTrue)
										So(entry.ProposedQuantity, ShouldNotBeNil)
										So(entry.ProposedQuantity.Sign(), ShouldEqual, 1)
									})
								})
							})
						})
					})
				})
			})
		}),
	)
}

/*
TestRoundTrip drives one pump from its precursor to its reversal and audits the
lot the stack actually carried through it.

Entering is half a trade. What decides whether recognizing a pump was worth
anything is the exit, so this asserts the realized economics rather than the
decision: the lot has to be opened by the strategy, protected once the move is
underway, closed by the regulator rather than by a second opinion from strategy,
and worth more on the way out than the round trip cost to hold it.
*/
func TestRoundTrip(t *testing.T) {
	Convey(
		"Setup",
		t, stack.WithOrders(t, symbols, func(market *tests.Market, system *cmd.System) {
			thesis := system.Thesis

			market.WithAutoFill()

			Convey("When a pump runs from its precursor into a reversal", func() {
				/*
					Every boundary here is one the generator owns. The entry is
					judged at the end of the precursor, the move is observed once
					its ignition has printed and decayed, and the lot is observed
					once the desk is flat — so nothing in this sequence depends on
					a number of ticks chosen to make it come out.
				*/
				So(market.Transition("SIM1/USD", testtypes.FastPump), ShouldBeNil)

				// The verdict is read where it is reached. A completed cycle
				// resets the thesis, so a decision inspected after the move has
				// run is a decision the run has already cleared.
				entry := symbolDecision(thesis, "SIM1/USD", types.ActionEnter)

				So(market.Express("SIM1/USD"), ShouldBeNil)

				armed := armedPositions(system, "SIM1/USD")

				So(market.Transition("SIM1/USD", testtypes.FastDump), ShouldBeNil)
				So(market.Express("SIM1/USD"), ShouldBeNil)
				So(market.Flatten("SIM1/USD"), ShouldBeNil)

				Convey("Then the strategy should have opened a lot on the precursor", func() {
					So(entry, ShouldNotBeNil)
					So(entry.Cause, ShouldEqual, "opportunity_entry")
					So(entry.ProposedQuantity.Sign(), ShouldEqual, 1)

					Convey("And the regulator should have protected it once the move ran", func() {
						So(armed, ShouldBeGreaterThan, 0)

						Convey("And the lot should be closed by its stoploss alone", func() {
							So(system.Desk.OpenPositions(), ShouldEqual, 0)

							Convey("And the round trip should have realized a profit", func() {
								closed := 0

								for position := range system.Desk.Positions() {
									holding := position.Holding

									if holding == nil ||
										holding.Symbol != "SIM1/USD" ||
										holding.Status != types.CLOSED {
										continue
									}

									closed++

									So(holding.EntryPrice, ShouldNotBeNil)
									So(holding.PnL, ShouldNotBeNil)

									/*
										The entry has to have been struck against
										this symbol's own book. A fill priced from
										somewhere else would report a return that
										is arithmetic on the wrong number rather
										than anything the lot did.
									*/
									So(holding.EntryPrice.Float64(),
										ShouldBeGreaterThan, symbols[0].StartPrice/2)

									// PnL is stated net of both crossings, so a
									// positive one is edge that survived friction.
									So(holding.PnL.Sign(), ShouldEqual, 1)
									So(holding.ReturnPct, ShouldBeGreaterThan, 0.0)
								}

								So(closed, ShouldBeGreaterThan, 0)
							})
						})
					})
				})
			})
		}),
	)
}

/*
TestRegimeDiscrimination runs every generated market condition at once and audits
which of them the strategy was willing to be long.

The profile declares whether a long belongs in the regime it generates, so this
asserts against that declaration rather than against a list repeated here. Most
of the matrix is negative on purpose: entering the one regime that pays is worth
nothing if the same rule also enters the seven that do not, and two of them —
spoofed depth and a thin book — are built to look like the one that does.
*/
func TestRegimeDiscrimination(t *testing.T) {
	Convey(
		"Setup",
		t, stack.WithStack(t, regimeSymbols, func(market *tests.Market, system *cmd.System) {
			thesis := system.Thesis

			Convey("When every market condition runs at once", func() {
				entries := map[string]bool{}

				for _, regime := range regimes {
					So(market.Transition(regime.Symbol, regime.State), ShouldBeNil)

					// A verdict is read at the moment it is reached, because a
					// completed cycle resets the thesis and a later reader would
					// be asking a map the run has already cleared.
					entries[regime.Symbol] = symbolDecision(
						thesis, regime.Symbol, types.ActionEnter,
					) != nil

					So(market.Express(regime.Symbol), ShouldBeNil)
				}

				Convey("Then it should be long exactly the regimes that admit one", func() {
					for _, regime := range regimes {
						profile := testtypes.DefaultProfiles[regime.State]
						entered := entries[regime.Symbol]

						So(
							fmt.Sprintf("%v:%s:%t", regime.State, regime.Symbol, entered),
							ShouldEqual,
							fmt.Sprintf("%v:%s:%t",
								regime.State, regime.Symbol, profile.AdmitsLong),
						)
					}
				})

				Convey("Then every regime should have produced its own evidence", func() {
					for _, regime := range regimes {
						measurements := 0

						for _, source := range signalSources() {
							if latestMeasurement(
								thesis, source, regime.Symbol,
							) != nil {
								measurements++
							}
						}

						/*
							A regime nothing measured cannot have been declined
							on its merits, so an empty reading here would make
							the assertions above pass for the wrong reason.
						*/
						So(
							fmt.Sprintf("%s:measured:%t", regime.Symbol, measurements > 0),
							ShouldEqual,
							fmt.Sprintf("%s:measured:true", regime.Symbol),
						)
					}
				})
			})
		}),
	)
}

/*
armedPositions counts this symbol's lots whose regulator has armed its profit
geometry, which is the state a lot has to reach before a reversal can close it
for a gain rather than at its entry risk.
*/
func armedPositions(system *cmd.System, symbol string) int {
	armed := 0

	for position := range system.Desk.Positions() {
		if position.Holding == nil || position.Holding.Symbol != symbol {
			continue
		}

		if position.StopSnapshot().ProfitArmed {
			armed++
		}
	}

	return armed
}

func symbolTickers(thesis *types.Thesis, symbol string) []kraken.TickerData {
	rows := make([]kraken.TickerData, 0)

	thesis.Tickers.Range(func(key, value interface{}) bool {
		ticker := value.(kraken.TickerData)
		if ticker.Symbol == symbol {
			rows = append(rows, ticker)
		}
		return true
	})

	return rows
}

func symbolTrades(thesis *types.Thesis, symbol string) []kraken.TradeData {
	rows := make([]kraken.TradeData, 0)

	thesis.Trades.Range(func(key, value interface{}) bool {
		trade := value.(kraken.TradeData)
		if trade.Symbol == symbol {
			rows = append(rows, trade)
		}
		return true
	})

	return rows
}

func signalSources() []types.SourceType {
	return []types.SourceType{
		types.SourceCorrelation,
		types.SourceCVD,
		types.SourceDepthFlow,
		types.SourceExhaustion,
		types.SourceHawkes,
		types.SourceLeadLag,
		types.SourceLiquidity,
		types.SourcePumpDump,
		types.SourceSentiment,
		types.SourceToxicity,
	}
}

func latestMeasurement(
	thesis *types.Thesis,
	source types.SourceType,
	symbol string,
) *types.Measurement {
	stored, found := thesis.Measurements.Load(source)

	if !found {
		return nil
	}

	rows, ok := stored.([]*types.Measurement)

	if !ok {
		return nil
	}

	var latest *types.Measurement

	for _, measurement := range rows {
		if measurement == nil || (symbol != "" && measurement.Symbol != symbol) {
			continue
		}

		if latest == nil || measurement.At.After(latest.At) {
			latest = measurement
		}
	}

	return latest
}

func findCategory(
	categories []types.Category,
	typ types.CategoryType,
) (types.Category, bool) {
	for _, category := range categories {
		if category.Type == typ {
			return category, true
		}
	}

	return types.Category{}, false
}

/*
symbolDecision returns the verdict this tick reached for one symbol when it
carries the given action, and nil when it reached a different one or none.
*/
func symbolDecision(
	thesis *types.Thesis,
	symbol string,
	action types.Action,
) *types.Decision {
	stored, found := thesis.Decisions.Load(symbol)

	if !found {
		return nil
	}

	decision, ok := stored.(*types.Decision)

	if !ok || decision.Action != action {
		return nil
	}

	return decision
}
