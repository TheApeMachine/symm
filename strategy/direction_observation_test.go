package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func TestDirectionalPredictorObserve(t *testing.T) {
	Convey("Given an envelope populated by every analytical observation family", t, func() {
		predictor, err := newDirectionalPredictor(directionalConfig{
			initialVariance:       1,
			forgettingFactor:      1,
			calibrationConfidence: 0.95,
		})
		So(err, ShouldBeNil)

		envelope := fullObservationEnvelope()
		err = predictor.observe(envelope)
		So(err, ShouldBeNil)

		state, err := predictor.state("TEST/USD")
		So(err, ShouldBeNil)

		families := map[string]int{}

		for key := range state.features {
			families[key.family]++
		}

		Convey("every family contributes separately identified facts", func() {
			So(families["measurement"], ShouldEqual, 1)
			So(families["perspective"], ShouldEqual, 1)
			So(families["category"], ShouldEqual, 4)
			So(families["opportunity"], ShouldEqual, 5)
			So(families["cognition"], ShouldEqual, 7)
			So(families["manifold"], ShouldEqual, 10)
			So(families["resonance"], ShouldEqual, 7)
		})
	})
}

func fullObservationEnvelope() *types.Envelope {
	metricSymbol := nmtypes.MustIntern("advisor.metric")
	resonanceSymbol := nmtypes.MustIntern("resonance.metric")
	measurement := data.NewMeasurement[float64](
		"measurement", "TEST/USD", "test", time.Unix(1, 0), time.Time{},
	)
	measurement.Maturity = 1
	measurement.PutMetric(data.Metric[float64]{Label: "raw", Raw: 1})

	envelope := types.NewEnvelope(types.EnvelopeTicker)
	envelope.TickerData.Symbol = "TEST/USD"
	envelope.Correlation = measurement
	envelope.Perspectives = []*types.Perspective{{
		Symbol: "TEST/USD",
		Kind:   types.KindCoordination,
		Count:  1,
		Readings: [types.PerspectiveMetricCapacity]types.MetricReading{{
			Metric: metricSymbol, Value: 1, Defined: true, Maturity: 1,
		}},
	}}
	envelope.Categories = []types.Category{{
		Symbol: "TEST/USD", Type: types.VerticalIgnition,
		Confidence: 0.9, Surprisal: 2, Strength: 1, Maturity: 1, Freshness: 1,
	}}
	envelope.Opportunities = []*types.OpportunityCandidate{{
		Symbol: "TEST/USD", Archetype: types.ArchetypeVerticalIgnition,
		Direction: types.DirectionLong, Provenance: types.ProvenanceCategory, Maturity: 1,
		Economics: &types.OpportunityEconomics{
			Calibrated: true, TransitionProbability: 0.8, ProfitFirst: 0.7, Uncertainty: 0.1,
		},
	}}
	envelope.Cognition = &types.Cognition{
		Symbol: "TEST/USD", Confidence: 0.8, ClassConfidence: 0.7,
		Contrast: 1, ContrastEvidence: 2, LookaheadScore: 3, InterpolatedSurprisal: 4,
		Classes:       []types.CognitionClass{{Name: "ignition", Probability: 0.8}},
		Contributions: []types.CognitionContribution{{Token: "coil", Bits: 2}},
	}
	envelope.Manifold = &types.ManifoldState{}
	envelope.Resonance = &types.ResonanceArtifact{Symbol: "TEST/USD", Confidence: 0.8}
	envelope.Resonance.Dynamics.Put(resonanceSymbol, 1)
	envelope.Resonance.Forecast = &types.ResonanceReturnForecast{Call: 1}

	return envelope
}
