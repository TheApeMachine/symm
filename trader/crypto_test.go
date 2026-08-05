package trader_test

import (
	"context"
	"fmt"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/tests"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/types"
)

var symbols = []*testtypes.Symbol{
	testtypes.NewSymbol("SIM1/USD", 64000.0, 42),
	testtypes.NewSymbol("SIM2/USD", 5432.193, 1337),
	testtypes.NewSymbol("SIM3/USD", 103.01234, 90210),
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
		t, tests.WithMarket(t, symbols, func(market *tests.Market) {
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
		t, tests.WithMarket(t, symbols, func(market *tests.Market) {
			Convey("When the market is transitioned to a fast pump", func() {
				So(market.Transition("SIM1/USD", testtypes.FastPump), ShouldBeNil)
				profile := testtypes.DefaultProfiles[testtypes.FastPump]
				expectation := profile.PrecursorExpectation(
					testtypes.DefaultProfiles[testtypes.Baseline],
				)
				thesis := market.TransitionThesis()

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
								categories := thesis.Categories["SIM1/USD"]

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
										decisions := market.Decisions()
										var entry *types.Decision

										for index := range decisions {
											decision := &decisions[index]

											if decision.Symbol == "SIM1/USD" &&
												decision.Action == types.ActionEnter {
												entry = decision
												break
											}
										}

										So(decisions, ShouldNotBeEmpty)
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

func symbolTickers(thesis *types.Thesis, symbol string) []kraken.TickerData {
	rows := make([]kraken.TickerData, 0)

	for _, ticker := range thesis.MarketTickers() {
		if ticker.Symbol == symbol {
			rows = append(rows, ticker)
		}
	}

	return rows
}

func symbolTrades(thesis *types.Thesis, symbol string) []kraken.TradeData {
	rows := make([]kraken.TradeData, 0)

	for _, trade := range thesis.MarketTrades() {
		if trade.Symbol == symbol {
			rows = append(rows, trade)
		}
	}

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
