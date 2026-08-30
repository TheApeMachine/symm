package cmd

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/types"
)

/*
TestCategoryMetricValueSensitivity follows the required behavioral-test pattern
for a newly-unstranded family (derivatives): feed a controlled measurement,
establish the semantic state, alter exactly one metric while holding all else
constant, and demonstrate the downstream category artifact changes accordingly.
It proves the integration is behavioral, not merely registration.
*/
func TestCategoryMetricValueSensitivity(t *testing.T) {
	Convey("Given one shared category solver", t, func() {
		_, categorySolver, _ := buildSemanticInstances(t)

		Convey("a stronger derivatives open-interest z-score produces a stronger LeveragedIgnition", func() {
			weak := types.NewEnvelope(types.EnvelopeFuturesTicker)
			weak.Derivatives = buildMetric("TEST/USD", "derivatives", time.Unix(100, 0), "open_interest_growth_zscore", 0.5)
			categorySolver.Step(weak)

			strong := types.NewEnvelope(types.EnvelopeFuturesTicker)
			strong.Derivatives = buildMetric("TEST/USD", "derivatives", time.Unix(101, 0), "open_interest_growth_zscore", 3.0)
			categorySolver.Step(strong)

			// Same category, but the stronger z-score yields stronger evidence.
			So(len(strong.Categories), ShouldBeGreaterThan, 0)
			So(strong.Categories[0].Type, ShouldEqual, types.LeveragedIgnition)

			weakStrength := categoryStrength(weak.Categories, types.LeveragedIgnition)
			strongStrength := categoryStrength(strong.Categories, types.LeveragedIgnition)
			So(strongStrength, ShouldBeGreaterThan, weakStrength)
		})

		Convey("contradictory evidence produces a meaningfully different result", func() {
			// Derivatives OI growth alone -> LeveragedIgnition.
			growthOnly := types.NewEnvelope(types.EnvelopeFuturesTicker)
			growthOnly.Derivatives = buildMetric("TEST/USD", "derivatives", time.Unix(100, 0), "open_interest_growth_zscore", 2.0)
			categorySolver.Step(growthOnly)
			So(growthOnly.Categories[0].Type, ShouldEqual, types.LeveragedIgnition)

			// Add hostile liquidation evidence (long deleveraging) on the same
			// symbol; the dominant regime must change because the two facts
			// constrain different regimes rather than reinforcing.
			liquidation := types.NewEnvelope(types.EnvelopeFuturesTicker)
			liquidation.Derivatives = buildMetric("TEST/USD", "derivatives", time.Unix(101, 0), "liquidation_share", 0.8)
			categorySolver.Step(liquidation)

			So(len(liquidation.Categories), ShouldBeGreaterThan, 0)
			// LongDeleveraging (liquidation_share is its schema leg) must now be
			// present with positive strength, not swallowed by LeveragedIgnition.
			So(categoryStrength(liquidation.Categories, types.LongDeleveraging), ShouldBeGreaterThan, 0.0)
		})

		Convey("an unrelated metric does not spuriously affect the relationship", func() {
			// Establish derivatives -> LeveragedIgnition.
			derivatives := types.NewEnvelope(types.EnvelopeFuturesTicker)
			derivatives.Derivatives = buildMetric("TEST/USD", "derivatives", time.Unix(100, 0), "open_interest_growth_zscore", 2.0)
			categorySolver.Step(derivatives)
			before := categoryStrength(derivatives.Categories, types.LeveragedIgnition)

			// An unrelated liquidity fact (spread) must not move the
			// LeveragedIgnition strength — it has its own category legs.
			liquidity := types.NewEnvelope(types.EnvelopeTicker)
			liquidity.Liquidity = buildMetric("TEST/USD", "liquidity", time.Unix(101, 0), "relative_spread", 0.001)
			categorySolver.Step(liquidity)
			after := categoryStrength(liquidity.Categories, types.LeveragedIgnition)

			// The derivatives coordinate is unchanged; LeveragedIgnition's
			// strength is driven only by its own leg, so it must stay equal.
			So(after, ShouldAlmostEqual, before, 1e-9)
		})
	})
}

/*
categoryStrength returns the strength of one category in a batch, or 0 if
absent. It is a test-only read of the solver's already-computed Strength.
*/
func categoryStrength(categories []types.Category, target types.CategoryType) float64 {
	for _, category := range categories {
		if category.Type == target {
			return category.Strength
		}
	}

	return 0
}
