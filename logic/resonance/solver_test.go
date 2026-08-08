package resonance

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
primeFeatures gives one symbol's feature standardizers a scale to score against.

The stage settles on standardized readings, and a standardizer answers zero
until it has prior moments, so a solver handed its first observation has nothing
to settle on and says so. A test that exercises what the stage does once it is
reading the market has to get it past that point first, which takes readings
that move: three ticks is what it costs to hold a mean, a spread, and a sample
scored against them.
*/
func primeFeatures(solver *Solver, symbol string, keys ...string) {
	for tick := range 3 {
		metrics := make(map[string]types.MetricSample, len(keys))

		for index, key := range keys {
			reading := 0.1 * float64(tick+index+1)
			metrics[key] = types.MetricSample{Normalized: &reading}
		}

		thesis := types.NewThesis(context.Background(), nil)
		thesis.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
			Source:  types.SourceLiquidity,
			Symbol:  symbol,
			Metrics: metrics,
		}})

		solver.Update(thesis)
	}
}

func TestNewSolver(t *testing.T) {
	Convey("Given the configured generative-model learning rate", t, func() {
		solver := NewSolver(t.Context(), nil, nil, testAlpha)

		Convey("It should initialize every symbol with that exact base pace", func() {
			So(solver.state("BTC/USD").alpha, ShouldEqual, testAlpha)
		})
	})

	Convey("Given an invalid configured learning rate", t, func() {
		solver := NewSolver(t.Context(), nil, nil, 0)
		err := solver.Update(types.NewThesis(t.Context(), nil))

		Convey("It should fail visibly before processing market state", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestExtractFeatures(t *testing.T) {
	Convey("Given the normalized fields published by the current Hawkes measurement", t, func() {
		buyArrival := 0.75
		sellArrival := 0.25
		eventSupport := 0.5
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Measurements.Store(types.SourceHawkes, []*types.Measurement{{
			Source: types.SourceHawkes,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricEventCount, types.SideNone): {
					Normalized: &eventSupport,
				},
				types.MetricKey(types.MetricArrivalRate, types.SideBuy): {
					Normalized: &buyArrival,
				},
				types.MetricKey(types.MetricArrivalRate, types.SideSell): {
					Normalized: &sellArrival,
				},
				types.MetricKey(types.MetricConditionalIntensity, types.SideBuy): {
					Raw: 2,
				},
			},
		}})

		features := (&Solver{}).extractFeatures(thesis)

		Convey("Then Resonance should consume the current metric identities without inventing an unready fit value", func() {
			So(features, ShouldResemble, map[string]map[string]float64{
				"BTC/USD": {
					"hawkes:BTC/USD:event_count":       eventSupport,
					"hawkes:BTC/USD:arrival_rate:buy":  buyArrival,
					"hawkes:BTC/USD:arrival_rate:sell": sellArrival,
				},
			})
		})
	})

	Convey("Given measurements with raw and normalized metric values", t, func() {
		normalized := 0.25
		notFinite := math.Inf(1)
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
			Source: types.SourceLiquidity,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				"relative_depth": {
					Raw:        25_000_000,
					Normalized: &normalized,
				},
				"raw_notional": {
					Raw: 25_000_000,
				},
				"invalid": {
					Raw:        1,
					Normalized: &notFinite,
				},
			},
		}})

		features := (&Solver{}).extractFeatures(thesis)

		Convey("Then only the finite normalized reading enters predictive coding", func() {
			So(features, ShouldResemble, map[string]map[string]float64{
				"BTC/USD": {
					"liquidity:BTC/USD:relative_depth": normalized,
				},
			})
		})
	})

	Convey("Given a relative reading whose peer rotates", t, func() {
		normalized := 0.5
		thesis := types.NewThesis(t.Context(), nil)

		reading := func(peer string) *types.Measurement {
			return &types.Measurement{
				Source: types.SourceLeadLag,
				Symbol: "BTC/USD",
				Peer:   peer,
				Metrics: map[string]types.MetricSample{
					"direction": {Raw: 1, Normalized: &normalized},
				},
			}
		}

		thesis.Measurements.Store(types.SourceLeadLag, []*types.Measurement{
			reading("ETH/USD"),
		})

		before := (&Solver{}).extractFeatures(thesis)

		thesis.Measurements.Store(types.SourceLeadLag, []*types.Measurement{
			reading("SOL/USD"),
		})

		after := (&Solver{}).extractFeatures(thesis)

		Convey("Then the feature identity should survive the rotation", func() {
			/*
				The peer is the counterpart a reading was taken against, not
				part of what is being measured. If it entered the identity,
				every rotation of the cross-section anchor would add an input
				dimension and reset the network that learns from them.
			*/
			So(after, ShouldResemble, before)
			So(after, ShouldResemble, map[string]map[string]float64{
				"BTC/USD": {
					"leadlag:BTC/USD:direction": normalized,
				},
			})
		})
	})
}

func TestUpdate(t *testing.T) {
	Convey("Given an initialized symbol whose upstream feature is absent this pass", t, func() {
		solver := NewSolver(t.Context(), nil, nil, testAlpha)
		symbol := "BTC/USD"
		primeFeatures(solver, symbol, "first", "second")
		first := 0.75
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
			Source: types.SourceLiquidity,
			Symbol: symbol,
			Metrics: map[string]types.MetricSample{
				"first": {Normalized: &first},
			},
		}})

		err := solver.Update(thesis)

		Convey("Then it should skip only that incomplete symbol without failing logic", func() {
			So(err, ShouldBeNil)
			_, published := thesis.Resonance.Load(symbol)
			So(published, ShouldBeFalse)
		})
	})

	Convey("Given an established schema from current Hawkes observations", t, func() {
		solver := NewSolver(t.Context(), nil, nil, testAlpha)
		symbol := "BTC/USD"

		for tick := range 3 {
			eventSupport := 0.25 + float64(tick)*0.1
			buyArrival := 0.6 + float64(tick)*0.05
			sellArrival := 0.4 - float64(tick)*0.05
			thesis := types.NewThesis(t.Context(), nil)
			thesis.Measurements.Store(types.SourceHawkes, []*types.Measurement{{
				Source: types.SourceHawkes,
				Symbol: symbol,
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricEventCount, types.SideNone): {
						Normalized: &eventSupport,
					},
					types.MetricKey(types.MetricArrivalRate, types.SideBuy): {
						Normalized: &buyArrival,
					},
					types.MetricKey(types.MetricArrivalRate, types.SideSell): {
						Normalized: &sellArrival,
					},
				},
			}})

			So(solver.Update(thesis), ShouldBeNil)
		}

		Convey("Then a pass carrying every current Hawkes feature should update and stamp", func() {
			eventSupport := 0.8
			buyArrival := 0.7
			sellArrival := 0.3
			thesis := types.NewThesis(t.Context(), nil)
			thesis.Measurements.Store(types.SourceHawkes, []*types.Measurement{{
				Source: types.SourceHawkes,
				Symbol: symbol,
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricEventCount, types.SideNone): {
						Normalized: &eventSupport,
					},
					types.MetricKey(types.MetricArrivalRate, types.SideBuy): {
						Normalized: &buyArrival,
					},
					types.MetricKey(types.MetricArrivalRate, types.SideSell): {
						Normalized: &sellArrival,
					},
				},
			}})

			err := solver.Update(thesis)

			So(err, ShouldBeNil)
			So(thesis.Readiness.Resonance, ShouldBeTrue)
			_, published := thesis.Resonance.Load(symbol)
			So(published, ShouldBeTrue)
		})
	})

	Convey("Given normalized predictive-coding features", t, func() {
		first := 0.25
		second := -0.5
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
			Source: types.SourceLiquidity,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				"first":  {Normalized: &first},
				"second": {Normalized: &second},
			},
		}})
		solver := NewSolver(t.Context(), make(chan []byte, 1), nil, testAlpha)
		primeFeatures(solver, "BTC/USD", "first", "second")
		thesis.AppendTicker(kraken.TickerData{
			Symbol:    "BTC/USD",
			Bid:       decimal.NewFromFloat64(99),
			Ask:       decimal.NewFromFloat64(101),
			Timestamp: time.Unix(1, 0),
		})

		err := solver.Update(thesis)

		Convey("Then surprise and energy are reported per input dimension", func() {
			So(err, ShouldBeNil)
			rowRaw, found := thesis.Resonance.Load("BTC/USD")
			So(found, ShouldBeTrue)

			row, ok := rowRaw.(types.ResonanceReading)
			So(ok, ShouldBeTrue)

			state := solver.state("BTC/USD")
			featureCount := float64(len(state.featureSchema))

			/*
				Surprise is an L2 norm over the input dimensions, so it grows as
				the square root of the feature count and takes the square root as
				its divisor. Energy is a sum of squared residuals, which grows
				linearly, so it takes the count itself. Each divisor has to match
				the units of what it normalizes, or the reading still carries the
				size of the schema.
			*/
			So(row.Surprise, ShouldAlmostEqual,
				state.manifold.ReconstructionError()/math.Sqrt(featureCount))

			/*
				PredictionEnergy rather than Energy. The latter adds latent decay
				and sparsity penalties whose magnitudes are set by the configured
				base pace, so it is not a pure reconstruction residual.
			*/
			So(row.Energy, ShouldAlmostEqual,
				state.manifold.PredictionEnergy()/featureCount)
			So(row.Samples, ShouldEqual, uint64(0))
			So(row.Forecast, ShouldNotBeNil)
			So(row.Forecast.Confidence, ShouldAlmostEqual, chanceForecastSkill)
			So(row.Forecast.ConfidenceReady, ShouldBeFalse)
			So(row.SkillEvidence, ShouldAlmostEqual, chanceForecastSkill)
			So(row.Verdict.Learning, ShouldEqual, "predicting")
			So(row.Layers, ShouldHaveLength, 3)
			So(row.Layers[0].State, ShouldHaveLength, len(state.featureSchema))
			So(row.Layers[0].Prediction, ShouldHaveLength, len(state.featureSchema))
			So(row.Layers[1].State, ShouldHaveLength, len(state.featureSchema)*2)
			So(row.Layers[1].Prediction, ShouldHaveLength, len(state.featureSchema)*2)
			So(row.Layers[2].State, ShouldHaveLength, len(state.featureSchema))
			So(row.Layers[2].Prediction, ShouldHaveLength, len(state.featureSchema))
		})

		Convey("Then an informative stage publishes a forecast its own curve supports", func() {
			first = 0.5
			second = -0.25
			thesis.AppendTicker(kraken.TickerData{
				Symbol:    "BTC/USD",
				Bid:       decimal.NewFromFloat64(100),
				Ask:       decimal.NewFromFloat64(102),
				Timestamp: time.Unix(2, 0),
			})

			err = solver.Update(thesis)
			So(err, ShouldBeNil)

			first = 0.75
			second = 0
			thesis.AppendTicker(kraken.TickerData{
				Symbol:    "BTC/USD",
				Bid:       decimal.NewFromFloat64(102),
				Ask:       decimal.NewFromFloat64(104),
				Timestamp: time.Unix(3, 0),
			})

			err = solver.Update(thesis)
			rowRaw, found := thesis.Resonance.Load("BTC/USD")
			So(found, ShouldBeTrue)

			row, ok := rowRaw.(types.ResonanceReading)
			So(ok, ShouldBeTrue)

			state := solver.state("BTC/USD")

			So(err, ShouldBeNil)

			/*
				This branch used to assert the opposite, and passed because the
				stage was settling on a vector of standardizer warmup zeros: the
				latent relaxed to the origin, its rollout retained nothing, and
				the forecast was published as invalid. That is a description of
				the warmup rather than of the retention, so what is pinned here
				now is the contract a stage reading actual features owes — a
				forecast whose horizon, curve and retention agree.
			*/
			So(row.Forecast, ShouldNotBeNil)
			So(row.Forecast.Validate(), ShouldBeNil)
			So(len(row.Forecast.Curve), ShouldEqual, row.Forecast.SupportedHorizon)
			So(len(row.Forecast.Retention), ShouldEqual, row.Forecast.SupportedHorizon)
			So(row.Forecast.Curve[0], ShouldAlmostEqual,
				state.manifold.TaskPrediction()[0])
			So(row.Forecast.ExpectedBasisPoints, ShouldNotEqual, 0.0)
			So(row.Forecast.ConfidenceReady, ShouldBeTrue)
			So(row.Forecast.PredictiveScale, ShouldBeGreaterThan, 0)
			So(state.targetSamples, ShouldEqual, 2)
		})
	})

	Convey("Given a ready feature observed at its learned mean", t, func() {
		solver := NewSolver(t.Context(), nil, nil, testAlpha)
		symbol := "BTC/USD"
		featureKey := "reading"

		for _, normalized := range []float64{0, 2} {
			thesis := types.NewThesis(t.Context(), nil)
			thesis.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
				Source: types.SourceLiquidity,
				Symbol: symbol,
				Metrics: map[string]types.MetricSample{
					featureKey: {Normalized: &normalized},
				},
			}})

			So(solver.Update(thesis), ShouldBeNil)
		}

		mean := 1.0
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
			Source: types.SourceLiquidity,
			Symbol: symbol,
			Metrics: map[string]types.MetricSample{
				featureKey: {Normalized: &mean},
			},
		}})

		err := solver.Update(thesis)

		Convey("Then the zero z-score remains a valid predictive-coding observation", func() {
			So(err, ShouldBeNil)

			stored, found := thesis.Resonance.Load(symbol)
			So(found, ShouldBeTrue)

			reading, ok := stored.(types.ResonanceReading)
			So(ok, ShouldBeTrue)
			So(reading.Stage, ShouldEqual, "resonance")
			So(reading.Verdict.Learning, ShouldEqual, "predicting")
			So(reading.Verdict.Tuning, ShouldEqual, "recursive least squares")
			So(reading.Forecast.Confidence, ShouldEqual, chanceForecastSkill)
			So(solver.state(symbol).manifold, ShouldNotBeNil)
			So(solver.state(symbol).extractor, ShouldNotBeNil)
		})
	})
}

func TestUpdateKeepsIndependentSymbolStates(t *testing.T) {
	Convey("Given a pending resonance sample from one symbol", t, func() {
		normalized := 0.25
		solver := NewSolver(t.Context(), make(chan []byte, 1), nil, testAlpha)
		primeFeatures(solver, "BTC/USD", "reading")
		primeFeatures(solver, "ETH/USD", "reading")

		first := types.NewThesis(t.Context(), nil)
		first.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
			Source: types.SourceLiquidity,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				"reading": {Normalized: &normalized},
			},
		}})
		first.AppendTicker(kraken.TickerData{
			Symbol:    "BTC/USD",
			Bid:       decimal.NewFromFloat64(99),
			Ask:       decimal.NewFromFloat64(101),
			Timestamp: time.Unix(1, 0),
		})

		So(solver.Update(first), ShouldBeNil)
		So(solver.state("BTC/USD").pendingAt.IsZero(), ShouldBeFalse)

		Convey("Then a later tick on a different target symbol must not train the same head sample", func() {
			second := types.NewThesis(t.Context(), nil)
			second.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
				Source: types.SourceLiquidity,
				Symbol: "ETH/USD",
				Metrics: map[string]types.MetricSample{
					"reading": {Normalized: &normalized},
				},
			}})
			second.AppendTicker(kraken.TickerData{
				Symbol:    "ETH/USD",
				Bid:       decimal.NewFromFloat64(199),
				Ask:       decimal.NewFromFloat64(201),
				Timestamp: time.Unix(2, 0),
			})

			So(solver.Update(second), ShouldBeNil)
			So(solver.state("BTC/USD").targetSamples, ShouldEqual, 0)
			So(solver.state("ETH/USD").targetSamples, ShouldEqual, 0)
			So(solver.state("ETH/USD").pendingAt.IsZero(), ShouldBeFalse)

			rowRaw, found := second.Resonance.Load("ETH/USD")
			So(found, ShouldBeTrue)

			row, ok := rowRaw.(types.ResonanceReading)
			So(ok, ShouldBeTrue)
			So(row.TargetSymbol, ShouldEqual, "ETH/USD")
		})
	})
}

/*
driveSolver runs the solver over a stream of resolvable ticker epochs and
returns it, so a test can inspect the reach it earned.

Each tick carries its own ticker timestamp, because the task head only resolves
a supervised sample when the market epoch advances. A stream that repeats one
epoch teaches the head nothing however long it runs.
*/
func driveSolver(ticks int) *Solver {
	solver := NewSolver(context.Background(), make(chan []byte, 1), nil, testAlpha)

	for tick := range ticks {
		/*
			The reading has to move. A feature repeated at one value has no
			scale to be scored against, so it standardizes to zero for as long
			as it is fed, and a head driven by it would be earning its reach
			from an input that never said anything.
		*/
		normalized := 0.5 + 0.25*math.Sin(float64(tick)/7.0)

		thesis := types.NewThesis(context.Background(), nil)
		thesis.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
			Source: types.SourceLiquidity,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				"reading": {Normalized: &normalized},
			},
		}})

		thesis.AppendTicker(kraken.TickerData{
			Symbol:    "BTC/USD",
			Bid:       decimal.NewFromFloat64(100 + float64(tick)*0.01),
			Ask:       decimal.NewFromFloat64(100.02 + float64(tick)*0.01),
			Timestamp: time.Unix(int64(tick), 0),
		})

		solver.Update(thesis)
	}

	return solver
}

/*
TestHorizonExtendsWhilePrecisionHolds pins the behaviour the predictive coding
stage exists to provide: a forecast window that grows as far as the head can
support and gives way when it cannot.
*/
func TestHorizonExtendsWhilePrecisionHolds(t *testing.T) {
	Convey("Given a head fed resolvable epochs until its precision settles", t, func() {
		solver := driveSolver(400)
		state := solver.state("BTC/USD")
		precision, hasPrecision := state.manifold.TaskPrecision()

		t.Logf("samples %d, precision %.3f, reach %d",
			state.targetSamples, precision, state.horizonReach)

		Convey("Then the head reports a precision it can be judged on", func() {
			So(hasPrecision, ShouldBeTrue)
			So(precision, ShouldBeGreaterThan, 0)
		})

		Convey("Then the reach grows well past a single step", func() {
			for range 4 {
				solver.horizon(state, 1.0)
			}

			So(state.targetSamples, ShouldBeGreaterThan, 0)
			So(state.horizonReach, ShouldBeGreaterThan, 1)
			So(state.horizonReach, ShouldBeLessThanOrEqualTo, int(state.targetSamples))
		})

		Convey("Then the published horizon does not outrun that reach", func() {
			horizon := solver.horizon(state, 1.0)

			So(horizon, ShouldBeGreaterThanOrEqualTo, 1)
			So(horizon, ShouldBeLessThanOrEqualTo, state.horizonReach)
		})
	})
}

func TestHorizonConfidenceCapsReach(t *testing.T) {
	Convey("Given a solver that has already earned multi-step reach", t, func() {
		solver := driveSolver(400)
		state := solver.state("BTC/USD")
		precision, hasPrecision := state.manifold.TaskPrecision()

		So(hasPrecision, ShouldBeTrue)
		So(precision, ShouldBeGreaterThan, 0)

		Convey("Then lower confidence contracts the remembered reach itself", func() {
			state.horizonReach = 10
			reach := state.horizonReach
			confidence := 0.4
			horizon := solver.horizon(state, confidence)
			expectedReach := min(int(state.targetSamples), reach+1)
			confidenceCap := max(1, int(math.Ceil(
				float64(expectedReach)*confidence,
			)))

			So(horizon, ShouldBeLessThan, reach)
			So(horizon, ShouldBeGreaterThanOrEqualTo, 1)
			So(horizon, ShouldBeLessThanOrEqualTo, confidenceCap)
			So(state.horizonReach, ShouldEqual, horizon)
		})
	})
}

func TestHorizonGrowsFromHighConfidence(t *testing.T) {
	Convey("Given high confidence at the shortest remembered reach", t, func() {
		solver := driveSolver(400)
		state := solver.state("BTC/USD")
		precision, hasPrecision := state.manifold.TaskPrecision()
		state.horizonReach = 1
		highConfidence := 0.95

		So(hasPrecision, ShouldBeTrue)
		So(precision, ShouldBeGreaterThanOrEqualTo, 0.5)

		Convey("Then it should grow the remembered reach immediately", func() {
			horizon := solver.horizon(state, highConfidence)

			So(horizon, ShouldEqual, 2)
			So(state.horizonReach, ShouldEqual, horizon)
		})
	})
}

/*
TestHorizonStartsShortWithoutSamples pins that reach is earned rather than
assumed. A head that has resolved no supervised sample has no basis for any
claim about the future.
*/
func TestHorizonStartsShortWithoutSamples(t *testing.T) {
	Convey("Given a solver whose head has resolved nothing", t, func() {
		solver := driveSolver(1)
		state := solver.state("BTC/USD")

		_, hasPrecision := state.manifold.TaskPrecision()

		Convey("Then it claims the shortest horizon", func() {
			So(hasPrecision, ShouldBeFalse)
			horizon := solver.horizon(state, 1.0)
			So(horizon, ShouldEqual, 1)
			So(state.horizonReach, ShouldEqual, 1)
		})
	})
}

/*
BenchmarkUpdate measures the two-symbol resonance pass at the feature width
observed in the full tick profile, including the focused symbol's hierarchy
snapshot. It exposes whether per-symbol fan-out helps or merely competes with
Gonum's own parallel matrix work.
*/
func BenchmarkUpdate(b *testing.B) {
	const featureCount = 48

	thesis := types.NewThesis(b.Context(), nil)
	solver := NewSolver(b.Context(), make(chan []byte, 1), nil, testAlpha)
	symbols := []string{"BTC/USD", "SIM2/USD"}

	for symbolIndex, symbol := range symbols {
		metrics := make(map[string]types.MetricSample, featureCount)

		for featureIndex := range featureCount {
			normalized := float64(featureIndex+symbolIndex) / featureCount
			metrics[fmt.Sprintf("feature_%d", featureIndex)] = types.MetricSample{
				Normalized: &normalized,
			}
		}

		thesis.Measurements.Store(types.SourceLiquidity, append(
			utils.Measurements(thesis, types.SourceLiquidity),
			&types.Measurement{
				Source:  types.SourceLiquidity,
				Symbol:  symbol,
				Metrics: metrics,
			},
		))
	}

	epoch := int64(1)
	b.ResetTimer()

	for b.Loop() {
		for _, symbol := range symbols {
			thesis.AppendTicker(kraken.TickerData{
				Symbol:    symbol,
				Bid:       decimal.NewFromFloat64(99),
				Ask:       decimal.NewFromFloat64(101),
				Timestamp: time.Unix(epoch, 0),
			})
		}

		if err := solver.Update(thesis); err != nil {
			b.Fatal(err)
		}

		epoch++
	}
}
