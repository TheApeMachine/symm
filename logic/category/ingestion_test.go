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
singleMetric builds one measurement peer for the category Step test.
*/
func singleMetric(source, symbol, metric string, at time.Time, value float64) *data.Measurement[float64] {
	return &data.Measurement[float64]{
		Label:  symbol,
		Source: source,
		At:     at,
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
*/
func TestStepIngestsStrandedFamilies(t *testing.T) {
	Convey("Given a category solver over the declared vocabulary", t, func() {
		solver := NewSolver(context.Background())
		at := time.Unix(100, 0)

		cases := []struct {
			name     string
			peer     *data.Measurement[float64]
			expected types.CategoryType
		}{
			{
				name:     "hawkes branching radius drives turbulent",
				peer:     singleMetric("hawkes", "TEST/USD", "branching_spectral_radius", at, 0.9),
				expected: types.Turbulent,
			},
			{
				name:     "pumpdump volume rate drives vertical ignition",
				peer:     singleMetric("pumpdump", "TEST/USD", "volume_rate", at, 0.8),
				expected: types.VerticalIgnition,
			},
			{
				name:     "toxicity ask fill drives liquidity vacuum",
				peer:     singleMetric("toxicity", "TEST/USD", "fill_fraction_zscore:ask", at, 0.7),
				expected: types.LiquidityVacuum,
			},
			{
				name:     "derivatives OI growth drives leveraged ignition",
				peer:     singleMetric("derivatives", "TEST/USD", "open_interest_growth_zscore", at, 0.9),
				expected: types.LeveragedIgnition,
			},
		}

		for _, testCase := range cases {
			Convey(testCase.name, func() {
				m := solver.Register()
				m.Label = "TEST/USD"
				m.At = at
				m.Peers = []*data.Measurement[float64]{testCase.peer}

				out := solver.Step(m)

				So(out, ShouldNotBeNil)
				So(out.Metrics[string(testCase.expected)], ShouldNotBeNil)
				So(out.Metrics[string(testCase.expected)].Raw, ShouldBeGreaterThan, 0)
			})
		}
	})
}
