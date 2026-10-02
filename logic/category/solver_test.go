package category

import (
	"context"
	"fmt"
	"github.com/theapemachine/symm/nomagique/runtime"
	"math"
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

	m := &data.Measurement[float64]{
		ID:       1,
		Source:   "cvd",
		Label:    symbol,
		At:       time.Unix(0, 1),
		Maturity: 0.9,
	}
	m.SetMetric("signed_net_fraction_zscore", metric)
	return m
}

func TestCategorySolverSingleSource(t *testing.T) {
	Convey("Given one eligible metric supporting aggressive_drive", t, func() {
		solver := NewSolver(context.Background(), data.NewArenaOwner(32))
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
		solver := NewSolver(context.Background(), data.NewArenaOwner(32))

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
		solver := NewSolver(context.Background(), data.NewArenaOwner(32))
		state := solver.symbolState("BTC/USD")

		for index := 0; index < 100; index++ {
			So(solver.accumulate(
				state, categoryMeasurement("BTC/USD", true, 0.8),
			), ShouldBeNil)
		}

		Convey("one coordinate is one current vote, not one hundred", func() {
			items := state.Coordinates()[coordinate{Source: "cvd", Metric: "signed_net_fraction_zscore"}]
			So(items.Affinity, ShouldEqual, 0.8)

			byCategory, _ := solver.aggregate(state)
			So(byCategory[types.AggressiveDrive], ShouldHaveLength, 1)
		})
	})
}

func TestCategorySolverCorroboration(t *testing.T) {
	Convey("Given two distinct coordinates supporting aggressive_drive", t, func() {
		solver := NewSolver(context.Background(), data.NewArenaOwner(32))
		state := solver.symbolState("BTC/USD")
		So(solver.accumulate(
			state, categoryMeasurement("BTC/USD", true, 0.64),
		), ShouldBeNil)
		// signed_net_fraction_divergence also maps to aggressive_drive.
		divergenceVal := 0.16
		m2 := &data.Measurement[float64]{
			ID:       2,
			Source:   "cvd",
			Label:    "BTC/USD",
			At:       time.Unix(0, 1),
			Maturity: 0.8,
		}
		m2.SetMetric("signed_net_fraction_divergence", data.Metric[float64]{
			Label:      "signed_net_fraction_divergence",
			Raw:        divergenceVal,
			Normalized: &divergenceVal,
		})
		So(solver.accumulate(state, m2), ShouldBeNil)

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
		solver := NewSolver(context.Background(), data.NewArenaOwner(32))
		stateA := solver.symbolState("A/USD")
		stateB := solver.symbolState("B/USD")

		So(solver.accumulate(
			stateA, categoryMeasurement("A/USD", true, 0.8),
		), ShouldBeNil)
		So(solver.accumulate(
			stateB, categoryMeasurement("B/USD", true, 0.9),
		), ShouldBeNil)

		Convey("each symbol holds only its own current evidence", func() {
			coordsA := stateA.Coordinates()
			coordsB := stateB.Coordinates()
			_, foundA := coordsA[coordinate{Source: "cvd", Metric: "signed_net_fraction_zscore"}]
			_, foundB := coordsB[coordinate{Source: "cvd", Metric: "signed_net_fraction_zscore"}]
			So(foundA, ShouldBeTrue)
			So(foundB, ShouldBeTrue)

			So(len(coordsA), ShouldEqual, 1)
			So(len(coordsB), ShouldEqual, 1)
		})
	})
}

func TestCategorySolverMissingEvidence(t *testing.T) {
	Convey("Given a symbol with no eligible evidence", t, func() {
		solver := NewSolver(context.Background(), data.NewArenaOwner(32))
		state := solver.symbolState("BTC/USD")

		Convey("classification is not measured", func() {
			_, measured := solver.aggregate(state)
			So(measured, ShouldBeFalse)
		})
	})
}

func TestCategorySolverDeterministicTie(t *testing.T) {
	Convey("Given equal evidence across categories", t, func() {
		solver := NewSolver(context.Background(), data.NewArenaOwner(32))

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
		solver := NewSolver(context.Background(), data.NewArenaOwner(32))
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
		solver := NewSolver(t.Context(), data.NewArenaOwner(32))
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
			item := state.Coordinates()[coordinate{
				Source: "cvd", Metric: "signed_net_fraction_zscore",
			}]
			So(item.Affinity, ShouldEqual, 0.4)
			So(item.At, ShouldResemble, second.At)
		})

		Convey("wall-clock distance does not invent a generic expiry", func() {
			trigger := &data.Measurement[float64]{
				ID: 3, Source: "unmapped", Label: "BTC/USD",
				At: time.Unix(86_400, 0),
			}
			categories := solver.StepMeasurement(trigger)

			So(categories, ShouldNotBeNil)
			So(categories[0].Type, ShouldEqual, types.AggressiveDrive)
			So(categories[0].At, ShouldResemble, trigger.At)
			So(categories[0].Freshness, ShouldEqual, 1.0)
		})
	})

	Convey("Given a failed signal measurement", t, func() {
		solver := NewSolver(t.Context(), data.NewArenaOwner(32))
		measurement := categoryMeasurement("BTC/USD", true, 0.8)
		measurement.Err = context.Canceled

		So(solver.StepMeasurement(measurement), ShouldBeNil)
		So(solver.Error(), ShouldNotBeNil)
		So(solver.Error().Error(), ShouldContainSubstring, "signal measurement failed")
	})

	Convey("Given multiple measurements where one failed but another succeeded", t, func() {
		solver := NewSolver(t.Context(), data.NewArenaOwner(32))
		failed := categoryMeasurement("BTC/USD", true, 0.8)
		failed.Err = context.Canceled

		divergenceVal := 0.5
		valid := &data.Measurement[float64]{
			ID:       4,
			Source:   "cvd",
			Label:    "BTC/USD",
			At:       time.Unix(0, 1),
			Maturity: 0.9,
		}
		valid.SetMetric("signed_net_fraction_divergence", data.Metric[float64]{
			Label:      "signed_net_fraction_divergence",
			Raw:        divergenceVal,
			Normalized: &divergenceVal,
		})

		categories := solver.stepMeasurements([]*data.Measurement[float64]{failed, valid})
		So(solver.Error(), ShouldBeNil)
		So(categories, ShouldNotBeNil)
	})

	Convey("Given the delayed MLN ticker observed in Hindsight", t, func() {
		solver := NewSolver(t.Context(), data.NewArenaOwner(32))
		newerTradeAt := time.Date(
			2026, time.September, 1, 22, 27, 48, 118_096_000, time.UTC,
		)
		olderTickerAt := time.Date(
			2026, time.September, 1, 22, 27, 48, 113_331_000, time.UTC,
		)
		trade := &data.Measurement[float64]{
			ID: 5, Source: "hawkes", Label: "MLN/USD", At: newerTradeAt,
		}
		trade.SetMetric("arrival_rate", data.Metric[float64]{
			Label: "arrival_rate",
			Raw:   9_208_790.233371342,
		})
		delayedTicker := &data.Measurement[float64]{
			ID: 6, Source: "correlation", Label: "MLN/USD",
			At: olderTickerAt,
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

	Convey("Given a peer whose From is after At (inverted interval)", t, func() {
		solver := NewSolver(t.Context(), data.NewArenaOwner(32))
		solver.Transition(runtime.READY)
		at := time.Unix(10, 0)
		good := categoryMeasurement("BTC/USD", true, 0.8)
		good.At = at
		good.From = at

		bad := &data.Measurement[float64]{
			ID:     -1,
			Source: "correlation:ticker",
			Label:  "BTC/USD",
			At:     at,
			From:   at.Add(time.Second), // inverted: interval begins after event
		}
		bad.SetMetric("cohort_signed_correlation", data.Metric[float64]{Raw: 0.5})

		So(solver.StepMeasurement(good), ShouldNotBeNil)
		categories := solver.stepMeasurements([]*data.Measurement[float64]{bad, good})

		Convey("Category soft-skips the inverted peer and stays READY", func() {
			So(solver.Error(), ShouldBeNil)
			So(solver.Status(), ShouldEqual, runtime.READY)
			So(categories, ShouldNotBeNil)
			So(solver.Version(), ShouldBeGreaterThan, uint64(0))
		})

		Convey("later Steps are not blocked by a before-READY flood", func() {
			again := solver.StepMeasurement(good)
			So(again, ShouldNotBeNil)
			So(solver.Status(), ShouldEqual, runtime.READY)
		})
	})
}

func TestSolverStep(t *testing.T) {
	Convey("Complete categories survive a multi-symbol registered observation", t, func() {
		solver := NewSolver(t.Context(), data.NewArenaOwner(32))
		solver.Transition(runtime.READY)
		measurement := solver.Register()
		measurement.Peers = []*data.Measurement[float64]{
			categoryMeasurement("ETH/USD", true, 0.4),
			categoryMeasurement("BTC/USD", true, 0.8),
		}
		result := solver.Step(measurement)
		batches, ok := result.Result.([][]types.Category)
		So(ok, ShouldBeTrue)
		So(len(batches), ShouldEqual, 2)
		for index, symbol := range []string{"BTC/USD", "ETH/USD"} {
			So(batches[index][0].Symbol, ShouldEqual, symbol)
			So(batches[index][0].At, ShouldEqual, time.Unix(0, 1))
			So(batches[index][0].Strength, ShouldBeGreaterThan, 0)
			So(batches[index][0].Supporting, ShouldNotBeEmpty)
		}
	})

	Convey("Given one measurement carrying multiple signal peers", t, func() {
		solver := NewSolver(t.Context(), data.NewArenaOwner(32))
		solver.Transition(runtime.READY)
		at := time.Unix(1, 0)
		m := solver.Register()
		m.Label, m.At, m.From = "BTC/USD", at, at

		cvd := data.NewMeasurement[float64]("cvd", nil)
		cvd.Label, cvd.At, cvd.From = "BTC/USD", at, at
		cvd.Maturity = 1
		cvd.SetMetric("signed_net_fraction_zscore", data.Metric[float64]{Label: "signed_net_fraction_zscore", Raw: 0.8})

		hawkes := data.NewMeasurement[float64]("hawkes", nil)
		hawkes.Label, hawkes.At, hawkes.From = "BTC/USD", at, at
		hawkes.Maturity = 1
		hawkes.SetMetric("arrival_rate", data.Metric[float64]{Label: "arrival_rate", Raw: 0.6})

		m.Peers = []*data.Measurement[float64]{cvd, hawkes}

		result := solver.Step(m)

		Convey("the observation commits one classification revision", func() {
			So(result, ShouldNotBeNil)
			So(solver.Version(), ShouldEqual, uint64(1))
			So(solver.Error(), ShouldBeNil)
		})
	})

	Convey("Given measurements with sub-100ms clock differences and negative z-scores", t, func() {
		solver := NewSolver(t.Context(), data.NewArenaOwner(32))
		solver.Transition(runtime.READY)
		at1 := time.Unix(10, 0)
		at2 := at1.Add(5 * time.Millisecond)

		m := solver.Register()
		m.Label, m.At, m.From = "BTC/USD", at1, at1

		cvd := data.NewMeasurement[float64]("cvd", nil)
		cvd.Label, cvd.At, cvd.From = "BTC/USD", at1, at1
		cvd.Maturity = 1
		cvd.SetMetric("signed_net_fraction_zscore", data.Metric[float64]{Label: "signed_net_fraction_zscore", Raw: -3.5})

		hawkes := data.NewMeasurement[float64]("hawkes", nil)
		hawkes.Label, hawkes.At, hawkes.From = "BTC/USD", at2, at2
		hawkes.Maturity = 1
		hawkes.SetMetric("arrival_rate", data.Metric[float64]{Label: "arrival_rate", Raw: 0.6})

		m.Peers = []*data.Measurement[float64]{cvd, hawkes}

		result := solver.Step(m)

		Convey("timestamp skew within tolerance is accepted and negative z-scores are retained", func() {
			So(solver.Error(), ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(result.GetMetric(string(types.AggressiveDrive)).Raw, ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkSolverStepMeasurement(b *testing.B) {
	for _, affinity := range []float64{0, 0.8} {
		b.Run(fmt.Sprint(affinity), func(b *testing.B) {
		solver := NewSolver(b.Context(), data.NewArenaOwner(32))
			defer func() {
				if err := solver.Close(); err != nil {
					b.Fatal(err)
				}
			}()
			measurement := categoryMeasurement("BTC/USD", true, affinity)
			b.ReportAllocs()

			for b.Loop() {
				categoryBenchmark = solver.StepMeasurement(measurement)
			}
		})
	}
}

func TestSolverBuildBatch(t *testing.T) {
	Convey("Category competition includes the symmetric one-pseudocount prior from specification section 20", t, func() {
		solver := NewSolver(t.Context(), data.NewArenaOwner(32))
		count := len(solver.categories)
		for _, strength := range []float64{0, 0.25, 1} {
			strengths := make([]float64, count)
			strengths[0] = strength
			batch, err := solver.buildBatch("CCD/USD", time.Unix(100, 0), strengths, nil)
			So(err, ShouldBeNil)
			So(len(batch), ShouldEqual, count)
			total := 0.0
			for index, category := range batch {
				expectedStrength := 0.0

				if index == 0 {
					expectedStrength = strength
				}
				expected := (expectedStrength + 1) / (float64(count) + strength)
				So(category.Confidence, ShouldAlmostEqual, expected)
				So(category.Surprisal, ShouldAlmostEqual, -math.Log2(expected))
				So(category.Strength, ShouldEqual, expectedStrength)
				So(category.Maturity, ShouldEqual, 0)
				total += category.Confidence
			}
			So(total, ShouldAlmostEqual, 1)

			if strength == 0 {
				So(batch[0].Uncertainty, ShouldAlmostEqual, 1)
			}
		}
	})
}

func TestSolverStepReadiness(t *testing.T) {
	Convey("An inactive pipeline node drops input before touching processing state", t, func() {
		node := &Solver{
			System: runtime.NewSystem(t.Context(), "readiness-test"),
			arena:  data.NewArenaOwner(32),
		}
		measurement := &data.Measurement[float64]{Label: "BTC/USD", SeqIdx: 7}
		for _, stage := range []runtime.Stage{runtime.INIT, runtime.WAITING, runtime.ERROR, runtime.FATAL} {
			node.Transition(stage)
			So(node.Step(measurement), ShouldBeNil)
			So(node.Status(), ShouldEqual, stage)
			So(measurement.SeqIdx, ShouldEqual, 7)
		}
	})
}

func BenchmarkSolverStep(b *testing.B) {
	solver := NewSolver(b.Context(), data.NewArenaOwner(32))
	solver.Transition(runtime.READY)
	measurement := solver.Register()
	measurement.Peers = []*data.Measurement[float64]{
		categoryMeasurement("BTC/USD", true, 0.8),
		categoryMeasurement("ETH/USD", true, 0.4),
	}
	b.ReportAllocs()

	for b.Loop() {
		solver.Step(measurement)

		if err := solver.Error(); err != nil {
			b.Fatal(err)
		}
	}
}

func TestSolverStepUnmeasured(t *testing.T) {
	Convey("Inputs without a category coordinate do not publish a registration as evidence", t, func() {
		solver := NewSolver(t.Context(), data.NewArenaOwner(32))
		solver.Transition(runtime.READY)
		measurement := solver.Register()
		peer := data.NewMeasurement[float64]("public", nil)
		peer.Label = "BTC/USD"
		peer.At = time.Unix(1, 0)
		measurement.Peers = []*data.Measurement[float64]{peer}
		So(solver.Step(measurement), ShouldBeNil)
		So(solver.Error(), ShouldBeNil)
	})
}

func TestStepWritesCategoryMetricsWithoutRegister(t *testing.T) {
	Convey("Given a live measurement without Register templates", t, func() {
		solver := NewSolver(t.Context(), data.NewArenaOwner(32))
		solver.Transition(runtime.READY)
		at := time.Unix(1, 0)
		m := data.NewMeasurement[float64]("websocket", nil)
		m.Label, m.At, m.From = "BTC/USD", at, at

		cvd := data.NewMeasurement[float64]("cvd", nil)
		cvd.Label, cvd.At, cvd.From = "BTC/USD", at, at
		cvd.Maturity = 1
		cvd.SetMetric("signed_net_fraction_zscore", data.Metric[float64]{Label: "signed_net_fraction_zscore", Raw: 0.8})

		hawkes := data.NewMeasurement[float64]("hawkes", nil)
		hawkes.Label, hawkes.At, hawkes.From = "BTC/USD", at, at
		hawkes.Maturity = 1
		hawkes.SetMetric("arrival_rate", data.Metric[float64]{Label: "arrival_rate", Raw: 0.6})

		m.Peers = []*data.Measurement[float64]{cvd, hawkes}
		result := solver.Step(m)
		So(result, ShouldNotBeNil)
		So(solver.Error(), ShouldBeNil)

		written := 0
		for _, cat := range solver.categories {
			if _, ok := result.LookupMetric(string(cat)); ok {
				written++
			}
		}
		So(written, ShouldBeGreaterThan, 0)
	})
}
