package trader_test

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

var symbols = []*testtypes.Symbol{
	testtypes.NewSymbol("SIM1/USD", 64000.0, 42),
	testtypes.NewSymbol("SIM2/USD", 5432.193, 1337),
	testtypes.NewSymbol("SIM3/USD", 103.01234, 90210),
	testtypes.NewSymbol("SIM4/USD", 0.00012345, 123456789),
	testtypes.NewSymbol("SIM5/USD", 987654321.0, 1),
	testtypes.NewSymbol("SIM6/USD", 50.0, 80085),
	testtypes.NewSymbol("SIM7/USD", 72.2123, 80085),
	testtypes.NewSymbol("SIM8/USD", 14.78192, 57391),
	testtypes.NewSymbol("SIM9/USD", 1987.442, 48120),
	testtypes.NewSymbol("SIM10/USD", 0.056781, 750394),
	testtypes.NewSymbol("SIM11/USD", 84567.99, 31415),
	testtypes.NewSymbol("SIM12/USD", 3.141592, 271828),
	testtypes.NewSymbol("SIM13/USD", 1250000.75, 600613),
	testtypes.NewSymbol("SIM14/USD", 0.987654, 918273),
	testtypes.NewSymbol("SIM15/USD", 456.789123, 112358),
}

var regimeSymbols = []*testtypes.Symbol{
	testtypes.NewSymbol("REG1/USD", 12.8754, 501),
	testtypes.NewSymbol("REG2/USD", 845.219, 7284),
	testtypes.NewSymbol("REG3/USD", 0.00458231, 15092),
	testtypes.NewSymbol("REG4/USD", 72193.44, 98231),
	testtypes.NewSymbol("REG5/USD", 18.999, 654321),
	testtypes.NewSymbol("REG6/USD", 2500000.0, 73),
	testtypes.NewSymbol("REG7/USD", 6.283185, 41256),
	testtypes.NewSymbol("REG8/USD", 0.87654321, 888001),
	testtypes.NewSymbol("REG9/USD", 15432.77, 99987),
	testtypes.NewSymbol("REG10/USD", 399.125, 210345),
	testtypes.NewSymbol("REG11/USD", 0.00000987, 7654321),
	testtypes.NewSymbol("REG12/USD", 910000.42, 184729),
	testtypes.NewSymbol("REG13/USD", 77.7777, 55555),
}

var regimes = []struct {
	Symbol string
	State  testtypes.MarketState
}{
	{"REG1/USD", testtypes.FastPump},
	{"REG2/USD", testtypes.FastDump},
	{"REG3/USD", testtypes.FlashCrash},
	{"REG4/USD", testtypes.LoadedLiquidity},
	{"REG5/USD", testtypes.Baseline},
	{"REG6/USD", testtypes.SidewaysChop},
	{"REG7/USD", testtypes.SlowPump},
	{"REG8/USD", testtypes.SlowDump},
	{"REG9/USD", testtypes.SpoofLiquidity},
	{"REG10/USD", testtypes.SpreadCompression},
	{"REG11/USD", testtypes.ThinLiquidity},
	{"REG12/USD", testtypes.VolatilitySpike},
	{"REG13/USD", testtypes.VolumeAbsorption},
}

func TestIntegration(t *testing.T) {
	Convey(
		"Setup",
		t, tests.WithStack(t, symbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
			thesis := system.Thesis

			Convey("When the market is transitioned to a fast pump", func() {
				subscription := system.Planner.Subscribe(
					"integration-decisions", types.NewSubscription[any](),
				)
				stopCollector, collected := collectDecisionBatches(
					t.Context(), subscription,
				)
				stopSnapshots, snapshots := collectIntegrationSnapshots(
					t.Context(), thesis, "SIM1/USD",
				)
				So(market.Transition("SIM1/USD", testtypes.FastPump), ShouldBeNil)
				stopCollector()
				stopSnapshots()
				decisionResult := <-collected
				So(decisionResult.err, ShouldBeNil)
				snapshotResult := <-snapshots
				So(snapshotResult.err, ShouldBeNil)
				snapshot := snapshotResult.snapshot
				entry := findDecision(
					decisionResult.decisions, "SIM1/USD", types.ActionEnter,
				)
				profile := testtypes.DefaultProfiles[testtypes.FastPump]
				expectation := profile.PrecursorExpectation(
					testtypes.DefaultProfiles[testtypes.Baseline],
				)

				Convey("Then canonical market history should satisfy the profile contract", func() {
					So(snapshot.tickers, ShouldNotBeEmpty)
					tickers := snapshot.tickers
					trades := snapshot.trades

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
							measurement := snapshot.measurements[source]

							So(fmt.Sprintf("%s:%t", source, measurement != nil),
								ShouldEqual, fmt.Sprintf("%s:true", source))
							So(measurement.At.IsZero(), ShouldBeFalse)
							So(measurement.Metrics, ShouldNotBeEmpty)
						}

						Convey("And pump evidence should meet the named precursor thresholds", func() {
							measurement := snapshot.measurements[types.SourcePumpDump]

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
								categories := snapshot.categories
								So(categories, ShouldNotBeEmpty)

								for _, expectedCategory := range expectation.Contract.Categories {
									category, found := findCategory(categories, expectedCategory)

									So(fmt.Sprintf("%s:%t", expectedCategory, found),
										ShouldEqual, fmt.Sprintf("%s:true", expectedCategory))
									So(category.Strength, ShouldBeGreaterThan, 0.0)
									So(category.Confidence, ShouldBeGreaterThan, 0.0)
								}

								Convey("And resonance should issue a positive forecast", func() {
									resonance, found := positiveResonance(snapshot.resonances)
									So(found, ShouldBeTrue)
									So(resonance.Samples, ShouldBeGreaterThan, 0)
									So(resonance.Forecast, ShouldNotBeNil)
									So(resonance.Forecast.Validate(), ShouldBeNil)
									So(resonance.Forecast.Curve, ShouldNotBeEmpty)
									So(resonance.Forecast.SupportedHorizon,
										ShouldEqual, len(resonance.Forecast.Curve))
									expectedReturn := resonance.Forecast.ExpectedReturn
									So(math.IsNaN(expectedReturn), ShouldBeFalse)
									So(expectedReturn, ShouldBeGreaterThan, 0.0)

									Convey("Then the planner should enter before ignition is sampled", func() {
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
		t, tests.WithOrders(t, symbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
			market.WithAutoFill()

			Convey("When a pump runs from its precursor into a reversal", func() {
				subscription := system.Planner.Subscribe(
					"roundtrip-decisions", types.NewSubscription[any](),
				)
				stopCollector, collected := collectDecisionBatches(
					t.Context(), subscription,
				)
				/*
					Every boundary here is one the generator owns. The entry is
					judged at the end of the precursor, the move is observed once
					its ignition has printed and decayed, and the lot is observed
					once the desk is flat — so nothing in this sequence depends on
					a number of ticks chosen to make it come out.
				*/
				So(market.Transition("SIM1/USD", testtypes.FastPump), ShouldBeNil)
				stopCollector()
				decisionResult := <-collected
				So(decisionResult.err, ShouldBeNil)
				entry := findDecision(
					decisionResult.decisions, "SIM1/USD", types.ActionEnter,
				)

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
							So(system.Desk.Holding("SIM1/USD"), ShouldEqual, 0)

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
of the matrix is negative on purpose: entering the regimes that pay is worth
nothing if the same rule also enters those that do not, and two of them — spoofed
depth and a thin book — are built to resemble directional opportunity.
*/
func TestRegimeDiscrimination(t *testing.T) {
	Convey(
		"Setup",
		t, tests.WithStack(t, regimeSymbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
			thesis := system.Thesis

			Convey("When every market condition runs at once", func() {
				subscription := system.Planner.Subscribe(
					"regime-decisions", types.NewSubscription[any](),
				)
				stopCollector, collected := collectDecisionBatches(
					t.Context(), subscription,
				)
				states := make(map[string]testtypes.MarketState, len(regimes))

				for _, regime := range regimes {
					states[regime.Symbol] = regime.State
				}

				So(market.TransitionAll(states), ShouldBeNil)
				stopCollector()
				decisionResult := <-collected
				So(decisionResult.err, ShouldBeNil)
				entries := map[string]bool{}

				for _, decision := range decisionResult.decisions {
					if decision.Action == types.ActionEnter {
						entries[decision.Symbol] = true
					}
				}

				Convey("Then it should admit the declared regimes from measured evidence", func() {
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
	}

	return armed
}

type integrationSnapshot struct {
	tickers      []kraken.TickerData
	trades       []kraken.TradeData
	measurements map[types.SourceType]*types.Measurement
	categories   []types.Category
	resonances   []types.ResonanceReading
}

type integrationSnapshotCollection struct {
	snapshot integrationSnapshot
	err      error
}

func collectIntegrationSnapshots(
	parent context.Context,
	thesis *types.Thesis,
	symbol string,
) (context.CancelFunc, <-chan integrationSnapshotCollection) {
	ctx, cancel := context.WithCancel(parent)
	completed := make(chan integrationSnapshotCollection, 1)

	go func() {
		collection := integrationSnapshotCollection{
			snapshot: integrationSnapshot{
				measurements: make(map[types.SourceType]*types.Measurement),
			},
		}

		for {
			updateIntegrationSnapshot(&collection.snapshot, thesis, symbol)

			select {
			case <-ctx.Done():
				if !integrationSnapshotReady(collection.snapshot) {
					collection.err = fmt.Errorf(
						"integration: no complete snapshot captured for %s", symbol,
					)
				}

				completed <- collection
				return
			default:
				runtime.Gosched()
			}
		}
	}()

	return cancel, completed
}

func updateIntegrationSnapshot(
	snapshot *integrationSnapshot,
	thesis *types.Thesis,
	symbol string,
) {
	tickers := symbolTickers(thesis, symbol)
	trades := symbolTrades(thesis, symbol)

	if len(tickers) > 0 && (len(snapshot.tickers) == 0 ||
		tickers[len(tickers)-1].Timestamp.After(
			snapshot.tickers[len(snapshot.tickers)-1].Timestamp,
		)) {
		snapshot.tickers = append([]kraken.TickerData(nil), tickers...)
	}

	if len(trades) > 0 && (len(snapshot.trades) == 0 ||
		trades[len(trades)-1].Timestamp.After(
			snapshot.trades[len(snapshot.trades)-1].Timestamp,
		)) {
		snapshot.trades = append([]kraken.TradeData(nil), trades...)
	}

	for _, source := range signalSources() {
		measurement := latestMeasurement(thesis, source, symbol)
		stored := snapshot.measurements[source]

		if measurement != nil && (stored == nil || measurement.At.After(stored.At)) {
			snapshot.measurements[source] = measurement
		}
	}

	categoriesRaw, categoriesReady := thesis.Categories.Load(symbol)

	if categories, ok := categoriesRaw.([]types.Category); categoriesReady && ok {
		snapshot.categories = mergeCategories(snapshot.categories, categories)
	}

	resonanceRaw, resonanceReady := thesis.Resonance.Load(symbol)

	if resonance, ok := resonanceRaw.(types.ResonanceReading); resonanceReady && ok &&
		resonance.Samples > 0 && (len(snapshot.resonances) == 0 ||
		resonance.At.After(snapshot.resonances[len(snapshot.resonances)-1].At)) {
		snapshot.resonances = append(snapshot.resonances, resonance)
	}
}

func integrationSnapshotReady(snapshot integrationSnapshot) bool {
	return len(snapshot.tickers) > 0 &&
		len(snapshot.trades) > 0 &&
		len(snapshot.measurements) == len(signalSources()) &&
		len(snapshot.categories) > 0 &&
		len(snapshot.resonances) > 0
}

func mergeCategories(
	observed []types.Category,
	current []types.Category,
) []types.Category {
	for _, category := range current {
		replaced := false

		for index := range observed {
			if observed[index].Type != category.Type {
				continue
			}

			observed[index] = category
			replaced = true
			break
		}

		if !replaced {
			observed = append(observed, category)
		}
	}

	return observed
}

func positiveResonance(
	readings []types.ResonanceReading,
) (types.ResonanceReading, bool) {
	for _, reading := range readings {
		if reading.Forecast == nil || reading.Forecast.ExpectedReturn <= 0 {
			continue
		}

		return reading, true
	}

	return types.ResonanceReading{}, false
}

type decisionCollection struct {
	decisions []types.Decision
	err       error
}

func collectDecisionBatches(
	parent context.Context,
	subscription *types.Subscription[any],
) (context.CancelFunc, <-chan decisionCollection) {
	ctx, cancel := context.WithCancel(parent)
	completed := make(chan decisionCollection, 1)

	go func() {
		collection := decisionCollection{}

		for {
			select {
			case emitted := <-subscription.Channel:
				decisions, ok := emitted.([]types.Decision)

				if !ok {
					collection.err = fmt.Errorf(
						"planner: expected detached decisions, got %T", emitted,
					)
					completed <- collection
					return
				}

				collection.decisions = append(collection.decisions, decisions...)
			case <-ctx.Done():
				for {
					select {
					case emitted := <-subscription.Channel:
						decisions, ok := emitted.([]types.Decision)

						if !ok {
							collection.err = fmt.Errorf(
								"planner: expected detached decisions, got %T",
								emitted,
							)
							completed <- collection
							return
						}

						collection.decisions = append(
							collection.decisions, decisions...,
						)
					default:
						completed <- collection
						return
					}
				}
			}
		}
	}()

	return cancel, completed
}

func findDecision(
	decisions []types.Decision,
	symbol string,
	action types.Action,
) *types.Decision {
	for index := range decisions {
		if decisions[index].Symbol == symbol && decisions[index].Action == action {
			return &decisions[index]
		}
	}

	return nil
}

func symbolTickers(thesis *types.Thesis, symbol string) []kraken.TickerData {
	rowsRaw, found := thesis.Tickers.Load(symbol)

	if !found {
		return nil
	}

	return rowsRaw.([]kraken.TickerData)
}

func symbolTrades(thesis *types.Thesis, symbol string) []kraken.TradeData {
	rowsRaw, found := thesis.Trades.Load(symbol)

	if !found {
		return nil
	}

	return rowsRaw.([]kraken.TradeData)
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
