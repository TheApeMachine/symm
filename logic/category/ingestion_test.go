package category

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

/*
singleMetric builds one envelope-field measurement for the category Step test.
*/
func singleMetric(source, symbol, metric string, at time.Time, value float64) *data.Measurement[float64] {
	return &data.Measurement[float64]{
		Label:   symbol,
		Source:  source,
		At:      at,
		Metrics: map[string]data.Metric[float64]{
			metric: {Raw: value},
		},
	}
}

/*
TestStepIngestsStrandedFamilies is the behavioral proof for the closed delta on
the Category side: a shared category solver's Step must fold a measurement from
each previously-ignored family (hawkes, pumpdump, toxicity, derivatives) into
its per-symbol evidence and drive that family's declared category verdict.

Each case feeds exactly one metric and asserts the resulting dominant regime is
the category that metric's CategorySchema leg declares — proving the value
flows, not merely that the field exists on the envelope.
*/
func TestStepIngestsStrandedFamilies(t *testing.T) {
	Convey("Given a category solver over the declared vocabulary", t, func() {
		solver := NewSolver(context.Background())
		at := time.Unix(100, 0)

		cases := []struct {
			name     string
			typeID   types.TypeID
			apply    func(*types.Envelope)
			expected types.CategoryType
		}{
			{
				name:   "hawkes branching radius drives turbulent",
				typeID: types.EnvelopeTrade,
				apply: func(envelope *types.Envelope) {
					envelope.Hawkes = singleMetric("hawkes", "TEST/USD", "branching_spectral_radius", at, 0.9)
				},
				expected: types.Turbulent,
			},
			{
				name:   "pumpdump volume rate drives vertical ignition",
				typeID: types.EnvelopeTicker,
				apply: func(envelope *types.Envelope) {
					envelope.PumpDump = singleMetric("pumpdump", "TEST/USD", "volume_rate", at, 0.8)
				},
				expected: types.VerticalIgnition,
			},
			{
				name:   "toxicity ask fill drives liquidity vacuum",
				typeID: types.EnvelopeLevel3,
				apply: func(envelope *types.Envelope) {
					envelope.Toxicity = singleMetric("toxicity", "TEST/USD", "fill_fraction_zscore:ask", at, 0.7)
				},
				expected: types.LiquidityVacuum,
			},
			{
				name:   "derivatives OI growth drives leveraged ignition",
				typeID: types.EnvelopeFuturesTicker,
				apply: func(envelope *types.Envelope) {
					envelope.Derivatives = singleMetric("derivatives", "TEST/USD", "open_interest_growth_zscore", at, 0.9)
				},
				expected: types.LeveragedIgnition,
			},
		}

		for _, testCase := range cases {
			Convey(testCase.name, func() {
				envelope := types.NewEnvelope(testCase.typeID)
				testCase.apply(envelope)

				solver.Step(envelope)

				So(len(envelope.Categories), ShouldBeGreaterThan, 0)
				So(envelope.Categories[0].Symbol, ShouldEqual, "TEST/USD")
				So(envelope.Categories[0].Type, ShouldEqual, testCase.expected)
			})
		}
	})
}
