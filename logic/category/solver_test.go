package category

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
categoryMeasurement builds one measurement carrying a single cvd metric with the
given normalized affinity, so the schema leg for aggressive_drive resolves.
*/
func categoryMeasurement(symbol string, normalized bool, value float64) *nmtypes.Measurement {
	metric := nmtypes.NewMetric("signed_net_fraction_zscore", value,
		nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
	)

	if normalized {
		metric = nmtypes.NewNormalizedMetric("signed_net_fraction_zscore", value, value,
			nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
		)
	}

	return &nmtypes.Measurement{
		ID:       "test",
		Source:   "cvd",
		Symbol:   symbol,
		At:       time.Unix(0, 1),
		Maturity: 0.9,
		Metrics: map[string]*nmtypes.Metric[float64]{
			"signed_net_fraction_zscore": metric,
		},
	}
}

func TestCategorySolverSingleSource(t *testing.T) {
	Convey("Given one eligible metric supporting aggressive_drive", t, func() {
		solver := NewSolver(context.Background())
		state := solver.symbolState("BTC/USD")
		solver.accumulate(state, categoryMeasurement("BTC/USD", true, 0.8))

		Convey("the dominant verdict is aggressive_drive", func() {
			batch, measured, err := solver.classify("BTC/USD", state)
			So(err, ShouldBeNil)
			So(measured, ShouldBeTrue)
			So(len(batch), ShouldBeGreaterThan, 0)
			So(batch[0].Type, ShouldEqual, types.AggressiveDrive)
		})
	})
}

func TestCategorySolverVersionMonotonic(t *testing.T) {
	Convey("Given a category solver committing several measured classifications", t, func() {
		solver := NewSolver(context.Background())

		Convey("the committed version is monotonic across transitions", func() {
			first := solver.StepMeasurement(categoryMeasurement("BTC/USD", true, 0.8))
			So(first, ShouldNotBeNil)

			versionAfterFirst := solver.Version()

			second := solver.StepMeasurement(categoryMeasurement("BTC/USD", true, 0.4))
			So(second, ShouldNotBeNil)

			versionAfterSecond := solver.Version()

			So(versionAfterFirst, ShouldBeGreaterThan, 0)
			So(versionAfterSecond, ShouldBeGreaterThan, versionAfterFirst)
		})
	})
}

func TestCategorySolverLatestStateReplacement(t *testing.T) {
	Convey("Given the same coordinate published many times", t, func() {
		solver := NewSolver(context.Background())
		state := solver.symbolState("BTC/USD")

		for index := 0; index < 100; index++ {
			solver.accumulate(state, categoryMeasurement("BTC/USD", true, 0.8))
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
		solver.accumulate(state, categoryMeasurement("BTC/USD", true, 0.64))
		// signed_net_fraction_divergence also maps to aggressive_drive.
		solver.accumulate(state, &nmtypes.Measurement{
			ID:       "test2",
			Source:   "cvd",
			Symbol:   "BTC/USD",
			At:       time.Unix(0, 1),
			Maturity: 0.8,
			Metrics: map[string]*nmtypes.Metric[float64]{
				"signed_net_fraction_divergence": nmtypes.NewNormalizedMetric(
					"signed_net_fraction_divergence", 0.16, 0.16,
					nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
				),
			},
		})

		Convey("strength is the geometric mean of the affinities", func() {
			byCategory, _ := solver.aggregate(state)
			strength := categoryStrength(byCategory[types.AggressiveDrive])
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

		solver.accumulate(stateA, categoryMeasurement("A/USD", true, 0.8))
		solver.accumulate(stateB, categoryMeasurement("B/USD", true, 0.9))

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
			_, measured, err := solver.classify("BTC/USD", state)
			So(err, ShouldBeNil)
			So(measured, ShouldBeFalse)
		})
	})
}

func TestCategorySolverDeterministicTie(t *testing.T) {
	Convey("Given equal evidence across categories", t, func() {
		solver := NewSolver(context.Background())

		Convey("batch ordering is stable across repeated builds", func() {
			first := solver.buildBatch("X/USD", make([]float64, len(solver.categories)), map[types.CategoryType][]evidenceItem{})
			second := solver.buildBatch("X/USD", make([]float64, len(solver.categories)), map[types.CategoryType][]evidenceItem{})

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
		solver.accumulate(state, categoryMeasurement("BTC/USD", true, 0.8))

		batch, measured, err := solver.classify("BTC/USD", state)
		So(err, ShouldBeNil)
		So(measured, ShouldBeTrue)
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
