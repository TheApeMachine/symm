package category

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

var categoryBenchmark []types.Category

/*
categoryMeasurement builds one measurement carrying a single cvd metric with the
given normalized affinity, so the schema leg for aggressive_drive resolves.
*/
func categoryMeasurement(symbol string, normalized bool, value float64) *data.Measurement[float64] {
	var normalizedVal *float64

	if normalized {
		normalizedVal = &value
	}

	metric := data.Metric[float64]{
		Label:      "signed_net_fraction_zscore",
		Raw:        value,
		Normalized: normalizedVal,
	}

	return &data.Measurement[float64]{
		ID:       "test",
		Source:   "cvd",
		Label:    symbol,
		At:       time.Unix(0, 1),
		Maturity: 0.9,
		Metrics: map[string]data.Metric[float64]{
			"signed_net_fraction_zscore": metric,
		},
	}
}

func TestCategorySolverSingleSource(t *testing.T) {
	Convey("Given one eligible metric supporting aggressive_drive", t, func() {
		solver := NewSolver(context.Background())
		state := solver.symbolState("BTC/USD")
		measurement := categoryMeasurement("BTC/USD", true, 0.8)
		So(solver.accumulate(state, measurement), ShouldBeNil)

		Convey("the dominant verdict is aggressive_drive", func() {
			byCategory, measured := solver.aggregate(state)
			So(measured, ShouldBeTrue)

			batch, err := solver.classify("BTC/USD", measurement.At, byCategory)
			So(err, ShouldBeNil)
			So(len(batch), ShouldBeGreaterThan, 0)
			So(batch[0].Type, ShouldEqual, types.AggressiveDrive)
		})
	})
}

func TestCategorySolverVersionMonotonic(t *testing.T) {
	Convey("Given a category solver committing several measured classifications", t, func() {
		solver := NewSolver(context.Background())

		Convey("the committed version is monotonic across transitions", func() {
			firstMeasurement := categoryMeasurement("BTC/USD", true, 0.8)
			firstMeasurement.At = time.Unix(1, 2)
			first := solver.StepMeasurement(firstMeasurement)
			So(first, ShouldNotBeNil)

			versionAfterFirst := solver.Version()

			secondMeasurement := categoryMeasurement("BTC/USD", true, 0.4)
			secondMeasurement.At = time.Unix(3, 4)
			second := solver.StepMeasurement(secondMeasurement)
			So(second, ShouldNotBeNil)

			versionAfterSecond := solver.Version()

			So(versionAfterFirst, ShouldBeGreaterThan, 0)
			So(versionAfterSecond, ShouldBeGreaterThan, versionAfterFirst)
			So(first[0].At, ShouldResemble, firstMeasurement.At)
			So(second[0].At, ShouldResemble, secondMeasurement.At)
		})

		Convey("a measurement without event time fails visibly", func() {
			measurement := categoryMeasurement("BTC/USD", true, 0.8)
			measurement.At = time.Time{}

			categories := solver.StepMeasurement(measurement)

			So(categories, ShouldBeNil)
			So(solver.Error(), ShouldNotBeNil)
			So(solver.Error().Error(), ShouldContainSubstring, "symbol and event time required")
			So(solver.Version(), ShouldEqual, 0)

			valid := categoryMeasurement("BTC/USD", true, 0.8)
			So(solver.StepMeasurement(valid), ShouldBeNil)
			So(solver.Version(), ShouldEqual, 0)
		})
	})
}

func TestCategorySolverLatestStateReplacement(t *testing.T) {
	Convey("Given the same coordinate published many times", t, func() {
		solver := NewSolver(context.Background())
		state := solver.symbolState("BTC/USD")

		for index := 0; index < 100; index++ {
			So(solver.accumulate(
				state, categoryMeasurement("BTC/USD", true, 0.8),
			), ShouldBeNil)
		}

		Convey("one coordinate is one current vote, not one hundred", func() {
			items := state.coordinates[coordinate{Source: "cvd", Metric: "signed_net_fraction_zscore"}]
			So(items.Affinity, ShouldEqual, 0.8)

			byCategory, _ := solver.aggregate(state)
			So(byCategory[types.AggressiveDrive], ShouldHaveLength, 1)
		})
	})
}

func TestCategorySolverCorroboration(t *testing.T) {
	Convey("Given two distinct coordinates supporting aggressive_drive", t, func() {
		solver := NewSolver(context.Background())
		state := solver.symbolState("BTC/USD")
		So(solver.accumulate(
			state, categoryMeasurement("BTC/USD", true, 0.64),
		), ShouldBeNil)
		// signed_net_fraction_divergence also maps to aggressive_drive.
		divergenceVal := 0.16
		So(solver.accumulate(state, &data.Measurement[float64]{
			ID:       "test2",
			Source:   "cvd",
			Label:    "BTC/USD",
			At:       time.Unix(0, 1),
			Maturity: 0.8,
			Metrics: map[string]data.Metric[float64]{
				"signed_net_fraction_divergence": {
					Label:      "signed_net_fraction_divergence",
					Raw:        divergenceVal,
					Normalized: &divergenceVal,
				},
			},
		}), ShouldBeNil)

		Convey("strength is the geometric mean of the affinities", func() {
			byCategory, _ := solver.aggregate(state)
			strength, err := categoryStrength(byCategory[types.AggressiveDrive])
			So(err, ShouldBeNil)
			// geomean(0.64, 0.16) = 0.32
			So(strength, ShouldAlmostEqual, 0.32)
		})
	})
}

func TestCategorySolverPerSymbolIsolation(t *testing.T) {
	Convey("Given interleaved measurements for two symbols", t, func() {
		solver := NewSolver(context.Background())
		stateA := solver.symbolState("A/USD")
		stateB := solver.symbolState("B/USD")

		So(solver.accumulate(
			stateA, categoryMeasurement("A/USD", true, 0.8),
		), ShouldBeNil)
		So(solver.accumulate(
			stateB, categoryMeasurement("B/USD", true, 0.9),
		), ShouldBeNil)

		Convey("each symbol holds only its own current evidence", func() {
			_, foundA := stateA.coordinates[coordinate{Source: "cvd", Metric: "signed_net_fraction_zscore"}]
			_, foundB := stateB.coordinates[coordinate{Source: "cvd", Metric: "signed_net_fraction_zscore"}]
			So(foundA, ShouldBeTrue)
			So(foundB, ShouldBeTrue)

			So(len(stateA.coordinates), ShouldEqual, 1)
			So(len(stateB.coordinates), ShouldEqual, 1)
		})
	})
}

func TestCategorySolverMissingEvidence(t *testing.T) {
	Convey("Given a symbol with no eligible evidence", t, func() {
		solver := NewSolver(context.Background())
		state := solver.symbolState("BTC/USD")

		Convey("classification is not measured", func() {
			_, measured := solver.aggregate(state)
			So(measured, ShouldBeFalse)
		})
	})
}

func TestCategorySolverDeterministicTie(t *testing.T) {
	Convey("Given equal evidence across categories", t, func() {
		solver := NewSolver(context.Background())

		Convey("batch ordering is stable across repeated builds", func() {
			at := time.Unix(1, 0)
			first, err := solver.buildBatch(
				"X/USD", at, make([]float64, len(solver.categories)),
				map[types.CategoryType][]evidenceItem{},
			)
			So(err, ShouldBeNil)

			second, err := solver.buildBatch(
				"X/USD", at, make([]float64, len(solver.categories)),
				map[types.CategoryType][]evidenceItem{},
			)
			So(err, ShouldBeNil)

			So(len(first), ShouldEqual, len(second))

			for index := range first {
				So(first[index].Type, ShouldEqual, second[index].Type)
				So(first[index].Confidence, ShouldAlmostEqual, second[index].Confidence)
			}
		})
	})
}

func TestCategorySolverUncertaintyIsDistributionLevel(t *testing.T) {
	Convey("Given a supported category", t, func() {
		solver := NewSolver(context.Background())
		state := solver.symbolState("BTC/USD")
		measurement := categoryMeasurement("BTC/USD", true, 0.8)
		So(solver.accumulate(state, measurement), ShouldBeNil)

		byCategory, measured := solver.aggregate(state)
		So(measured, ShouldBeTrue)

		batch, err := solver.classify("BTC/USD", measurement.At, byCategory)
		So(err, ShouldBeNil)
		So(len(batch), ShouldBeGreaterThan, 0)

		Convey("every entry carries the same distribution-level uncertainty", func() {
			first := batch[0].Uncertainty

			for _, entry := range batch[1:] {
				So(entry.Uncertainty, ShouldAlmostEqual, first)
			}

			Convey("uncertainty is not 1 - confidence", func() {
				So(batch[0].Uncertainty, ShouldNotAlmostEqual, 1.0-batch[0].Confidence)
			})
		})
	})
}

func TestSolverStepMeasurement(t *testing.T) {
	Convey("Given one coordinate updated in committed observation order", t, func() {
		solver := NewSolver(t.Context())
		first := categoryMeasurement("BTC/USD", true, 0.8)
		first.At = time.Unix(3, 0)
		second := categoryMeasurement("BTC/USD", true, 0.4)
		second.At = time.Unix(2, 0)

		So(solver.StepMeasurement(first), ShouldNotBeNil)

		Convey("a later commit with older event provenance becomes current", func() {
			categories := solver.StepMeasurement(second)

			So(categories, ShouldNotBeNil)
			So(categories[0].Type, ShouldEqual, types.AggressiveDrive)
			So(categories[0].At, ShouldResemble, second.At)
			So(categories[0].Freshness, ShouldEqual, 1.0)
			So(solver.Error(), ShouldBeNil)

			state := solver.symbolState("BTC/USD")
			item := state.coordinates[coordinate{
				Source: "cvd", Metric: "signed_net_fraction_zscore",
			}]
			So(item.Affinity, ShouldEqual, 0.4)
			So(item.At, ShouldResemble, second.At)
		})

		Convey("wall-clock distance does not invent a generic expiry", func() {
			trigger := &data.Measurement[float64]{
				ID: "trigger", Source: "unmapped", Label: "BTC/USD",
				At: time.Unix(86_400, 0), Metrics: map[string]data.Metric[float64]{},
			}
			categories := solver.StepMeasurement(trigger)

			So(categories, ShouldNotBeNil)
			So(categories[0].Type, ShouldEqual, types.AggressiveDrive)
			So(categories[0].At, ShouldResemble, trigger.At)
			So(categories[0].Freshness, ShouldEqual, 1.0)
		})
	})

	Convey("Given a failed signal measurement", t, func() {
		solver := NewSolver(t.Context())
		measurement := categoryMeasurement("BTC/USD", true, 0.8)
		measurement.Err = context.Canceled

		So(solver.StepMeasurement(measurement), ShouldBeNil)
		So(solver.Error(), ShouldNotBeNil)
		So(solver.Error().Error(), ShouldContainSubstring, "signal measurement failed")
	})

	Convey("Given multiple measurements where one failed but another succeeded", t, func() {
		solver := NewSolver(t.Context())
		failed := categoryMeasurement("BTC/USD", true, 0.8)
		failed.Err = context.Canceled

		divergenceVal := 0.5
		valid := &data.Measurement[float64]{
			ID:       "valid-test",
			Source:   "cvd",
			Label:    "BTC/USD",
			At:       time.Unix(0, 1),
			Maturity: 0.9,
			Metrics: map[string]data.Metric[float64]{
				"signed_net_fraction_divergence": {
					Label:      "signed_net_fraction_divergence",
					Raw:        divergenceVal,
					Normalized: &divergenceVal,
				},
			},
		}

		categories := solver.stepMeasurements([]*data.Measurement[float64]{failed, valid})
		So(solver.Error(), ShouldBeNil)
		So(categories, ShouldNotBeNil)
	})

	Convey("Given the delayed MLN ticker observed in Hindsight", t, func() {
		solver := NewSolver(t.Context())
		newerTradeAt := time.Date(
			2026, time.September, 1, 22, 27, 48, 118_096_000, time.UTC,
		)
		olderTickerAt := time.Date(
			2026, time.September, 1, 22, 27, 48, 113_331_000, time.UTC,
		)
		trade := &data.Measurement[float64]{
			ID: "hawkes", Source: "hawkes", Label: "MLN/USD", At: newerTradeAt,
			Metrics: map[string]data.Metric[float64]{
				"arrival_rate": {
					Label: "arrival_rate",
					Raw:   9_208_790.233371342,
				},
			},
		}
		delayedTicker := &data.Measurement[float64]{
			ID: "correlation", Source: "correlation", Label: "MLN/USD",
			At: olderTickerAt, Metrics: map[string]data.Metric[float64]{},
		}

		So(solver.StepMeasurement(trade), ShouldNotBeNil)
		categories := solver.StepMeasurement(delayedTicker)

		Convey("the already-known trade fact survives the older provenance", func() {
			So(categories, ShouldNotBeNil)
			So(categories[0].At, ShouldResemble, olderTickerAt)
			So(solver.Error(), ShouldBeNil)
			So(solver.Version(), ShouldEqual, uint64(2))
		})
	})
}

func TestSolverStep(t *testing.T) {
	Convey("Given one envelope carrying multiple signal measurements", t, func() {
		solver := NewSolver(t.Context())
		at := time.Unix(1, 0)
		envelope := types.NewEnvelope(types.EnvelopeTrade)
		envelope.CVD = data.NewMeasurement[float64](
			"cvd", "BTC/USD", "cvd", at, at,
		)
		envelope.CVD.Maturity = 1
		envelope.CVD.PutMetric(data.Metric[float64]{
			Label: "signed_net_fraction_zscore", Raw: 0.8,
		})
		envelope.Hawkes = data.NewMeasurement[float64](
			"hawkes", "BTC/USD", "hawkes", at, at,
		)
		envelope.Hawkes.Maturity = 1
		envelope.Hawkes.PutMetric(data.Metric[float64]{
			Label: "arrival_rate", Raw: 0.6,
		})

		result := solver.Step(envelope)

		Convey("the observation commits one classification revision", func() {
			So(result, ShouldEqual, envelope)
			So(result.Categories, ShouldNotBeNil)
			So(solver.Version(), ShouldEqual, uint64(1))
			So(solver.Error(), ShouldBeNil)
		})
	})
}

func BenchmarkSolverStepMeasurement(b *testing.B) {
	solver := NewSolver(b.Context())
	measurement := categoryMeasurement("BTC/USD", true, 0.8)
	b.ReportAllocs()

	for b.Loop() {
		categoryBenchmark = solver.StepMeasurement(measurement)
	}
}
