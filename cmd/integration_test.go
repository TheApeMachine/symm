package cmd

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/relation"
	"github.com/theapemachine/symm/types"
)

/*
TestGraphIngestsDerivatives proves the closed delta: a derivatives measurement
(produced on a futures Workload, which now mounts the shared semantic core)
reaches the ONE authoritative Influence Graph. Previously graph.Step ignored
envelope.Derivatives, so the catalog's derivatives coordinates (basis_zscore,
open_interest_growth_zscore, liquidation_notional_rate) could never be
estimated against any other metric.
*/
func TestGraphIngestsDerivatives(t *testing.T) {
	Convey("Given one shared graph solver", t, func() {
		graphSolver, _, _ := buildSemanticInstances(t)

		envelope := types.NewEnvelope(types.EnvelopeFuturesTicker)
		envelope.Derivatives = buildMetric("TEST/USD", "derivatives", time.Unix(100, 0), "basis_zscore", 2.5)

		graphSolver.Step(envelope)

		Convey("the derivatives basis_zscore coordinate enters the coordinate store", func() {
			found := false
			graphSolver.Store().RangeCoordinatesForSymbol("TEST/USD", func(coordinate relation.Coordinate) bool {
				if coordinate.Source == "derivatives" && coordinate.Metric == "basis_zscore" {
					found = true
					return false
				}
				return true
			})

			So(found, ShouldBeTrue)
		})
	})
}

/*
TestCategoryIngestsDerivativesAndHawkes proves the closed delta for the Category
side: a derivatives measurement and a hawkes measurement (both previously
ignored by category.Step) now flow into the shared per-symbol evidence state and
drive their declared category verdicts.
*/
func TestCategoryIngestsDerivativesAndHawkes(t *testing.T) {
	Convey("Given one shared category solver", t, func() {
		_, categorySolver, _ := buildSemanticInstances(t)

		Convey("derivatives open_interest_growth_zscore drives LeveragedIgnition", func() {
			envelope := types.NewEnvelope(types.EnvelopeFuturesTicker)
			envelope.Derivatives = buildMetric("TEST/USD", "derivatives", time.Unix(100, 0), "open_interest_growth_zscore", 2.0)

			categorySolver.Step(envelope)

			So(len(envelope.Categories), ShouldBeGreaterThan, 0)
			So(envelope.Categories[0].Type, ShouldEqual, types.LeveragedIgnition)
		})

		Convey("hawkes branching_spectral_radius drives Turbulent", func() {
			envelope := types.NewEnvelope(types.EnvelopeTrade)
			envelope.Hawkes = buildMetric("TEST/USD", "hawkes", time.Unix(100, 0), "branching_spectral_radius", 0.9)

			categorySolver.Step(envelope)

			So(len(envelope.Categories), ShouldBeGreaterThan, 0)
			So(envelope.Categories[0].Type, ShouldEqual, types.Turbulent)
		})

		Convey("toxicity fill_fraction_zscore:bid drives LiquidityVacuum", func() {
			envelope := types.NewEnvelope(types.EnvelopeTrade)
			envelope.Toxicity = buildMetric("TEST/USD", "toxicity", time.Unix(100, 0), "fill_fraction_zscore:bid", 1.5)

			categorySolver.Step(envelope)

			So(len(envelope.Categories), ShouldBeGreaterThan, 0)
			So(envelope.Categories[0].Type, ShouldEqual, types.LiquidityVacuum)
		})
	})
}

/*
TestDerivativesConditionsBookImbalance is a cross-family behavioral test: the
Influence Graph's declared catalog relates derivatives/basis_zscore (Tier 1) to
depthflow/book_imbalance (Tier 2) via the default causal schema. Feed both
measurements through the REAL shared graph solver and assert the cross-family
candidate edge is scheduled/estimated — proving derivatives now participates in
the Influence Graph rather than being stranded on the futures ring.
*/
func TestDerivativesConditionsBookImbalance(t *testing.T) {
	Convey("Given the shared graph solver over the real causal schema", t, func() {
		graphSolver, _, _ := buildSemanticInstances(t)

		for index := 1; index <= 4; index++ {
			derivativesEnvelope := types.NewEnvelope(types.EnvelopeFuturesTicker)
			derivativesEnvelope.Derivatives = buildMetric(
				"TEST/USD", "derivatives", time.Unix(int64(index), 0),
				"basis_zscore", float64(index),
			)
			graphSolver.Step(derivativesEnvelope)

			depthflowEnvelope := types.NewEnvelope(types.EnvelopeLevel3)
			depthflowEnvelope.DepthFlow = buildMetric(
				"TEST/USD", "depthflow", time.Unix(int64(index), 0),
				"book_imbalance", float64(index)*0.5,
			)
			graphSolver.Step(depthflowEnvelope)
		}

		Convey("the catalog's basis_zscore → book_imbalance relation is scheduled", func() {
			crossFamilyScheduled := false

			for _, candidate := range graphSolver.Graph().Candidates() {
				if candidate.Source.Source == "derivatives" && candidate.Source.Metric == "basis_zscore" &&
					candidate.Target.Source == "depthflow" && candidate.Target.Metric == "book_imbalance" {
					crossFamilyScheduled = true
				}
			}

			So(crossFamilyScheduled, ShouldBeTrue)
		})
	})
}
