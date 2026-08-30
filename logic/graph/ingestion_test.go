package graph

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/relation"
	"github.com/theapemachine/symm/types"
)

/*
singleMetric builds one envelope-field measurement carrying a single metric,
scoped to the given source. It is the graph-test mirror of the production
signal kernels: one Measurement per signal, keyed by (Source, metric).
*/
func singleMetric(source, symbol, metric string, at time.Time, value float64) *data.Measurement[float64] {
	return &data.Measurement[float64]{
		Label:   symbol,
		Source:  source,
		At:      at,
		From:    at.Add(-time.Second),
		Metrics: map[string]data.Metric[float64]{
			metric: {Raw: value},
		},
	}
}

/*
TestStepIngestsEverySignalFamily is the behavioral proof for the closed delta: a
single shared graph solver's Step must fold a measurement from EACH signal
family into the coordinate store — including the four that were previously
ignored (hawkes, pumpdump, toxicity, derivatives). With the ingestion severed,
the associated coordinates never enter the store and the Influence Graph's
declared catalog relationships (e.g. derivatives → depthflow, cvd → toxicity)
could never be estimated.
*/
func TestStepIngestsEverySignalFamily(t *testing.T) {
	Convey("Given a graph solver", t, func() {
		solver := NewSolver(context.Background(), 1, 2048, nil, 1)
		at := time.Unix(100, 0)

		envelope := &types.Envelope{
			Correlation:  singleMetric("correlation", "TEST/USD", "signed_correlation", at, 0.5),
			LeadLag:      singleMetric("leadlag", "TEST/USD", "best_lag_correlation", at, 0.4),
			Liquidity:    singleMetric("liquidity", "TEST/USD", "relative_spread", at, 0.01),
			Sentiment:    singleMetric("sentiment", "TEST/USD", "breadth_zscore", at, 0.3),
			CVD:          singleMetric("cvd", "TEST/USD", "signed_net_fraction_zscore", at, 0.6),
			DepthFlow:    singleMetric("depthflow", "TEST/USD", "book_imbalance", at, 0.2),
			Morphology:   singleMetric("morphology", "TEST/USD", "bid_ask_shape_distance", at, 0.1),
			Hawkes:       singleMetric("hawkes", "TEST/USD", "branching_spectral_radius", at, 0.7),
			PumpDump:     singleMetric("pumpdump", "TEST/USD", "volume_bar_quantity", at, 0.8),
			Toxicity:     singleMetric("toxicity", "TEST/USD", "fill_fraction_zscore:bid", at, 0.9),
			Derivatives:  singleMetric("derivatives", "TEST/USD", "basis_zscore", at, 1.1),
		}

		solver.Step(envelope)

		expected := map[string]bool{
			"correlation/signed_correlation":        false,
			"leadlag/best_lag_correlation":          false,
			"liquidity/relative_spread":             false,
			"sentiment/breadth_zscore":              false,
			"cvd/signed_net_fraction_zscore":        false,
			"depthflow/book_imbalance":              false,
			"morphology/bid_ask_shape_distance":     false,
			"hawkes/branching_spectral_radius":      false,
			"pumpdump/volume_bar_quantity":          false,
			"toxicity/fill_fraction_zscore:bid":     false,
			"derivatives/basis_zscore":              false,
		}

		solver.Store().RangeCoordinatesForSymbol("TEST/USD", func(coordinate relation.Coordinate) bool {
			key := coordinate.Source + "/"

			if coordinate.Side != "" {
				key += coordinate.Metric + ":" + coordinate.Side
			} else {
				key += coordinate.Metric
			}

			if _, declared := expected[key]; declared {
				expected[key] = true
			}

			return true
		})

		for _, found := range expected {
			So(found, ShouldBeTrue)
		}
	})
}
