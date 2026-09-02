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
			initialVariance:  1,
			forgettingFactor: 1,
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
			So(families["category"], ShouldEqual, 4)
			So(families["opportunity"], ShouldEqual, 5)
			So(families["cognition"], ShouldEqual, 7)
			So(families["manifold"], ShouldEqual, 10)
			So(families["resonance"], ShouldEqual, 7)
		})
	})

	Convey("Given a ticker whose Resonance stage produced no artifact", t, func() {
		predictor, err := newDirectionalPredictor(directionalConfig{
			initialVariance: 1, forgettingFactor: 1,
		})
		So(err, ShouldBeNil)
		envelope := types.NewEnvelope(types.EnvelopeTicker)
		envelope.TickerData.Symbol = "TEST/USD"
		envelope.TickerData.Timestamp = time.Unix(1, 0)

		err = predictor.observe(envelope)

		Convey("the old horizon cannot survive the missing stage", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "requires its resonance artifact")
		})
	})

	Convey("Given a ticker carrying a different Resonance observation", t, func() {
		predictor, err := newDirectionalPredictor(directionalConfig{
			initialVariance: 1, forgettingFactor: 1,
		})
		So(err, ShouldBeNil)
		envelope := fullObservationEnvelope()
		envelope.Resonance.Symbol = "OTHER/USD"

		err = predictor.observe(envelope)

		Convey("the ticker cannot reuse one symbol's horizon for another", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "matching resonance identity")
			So(predictor.states, ShouldBeEmpty)
		})
	})

	Convey("Given a ticker carrying Resonance from a different event time", t, func() {
		predictor, err := newDirectionalPredictor(directionalConfig{
			initialVariance: 1, forgettingFactor: 1,
		})
		So(err, ShouldBeNil)
		envelope := fullObservationEnvelope()
		envelope.Resonance.At = envelope.TickerData.Timestamp.Add(-time.Nanosecond)

		err = predictor.observe(envelope)

		Convey("the old horizon cannot cross the ticker boundary", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "matching resonance identity")
			So(predictor.states, ShouldBeEmpty)
		})
	})
}

func TestDirectionalPredictorObserveResonance(t *testing.T) {
	Convey("Given Resonance horizon evidence", t, func() {
		predictor, err := newDirectionalPredictor(directionalConfig{
			initialVariance:  1,
			forgettingFactor: 1,
		})
		So(err, ShouldBeNil)

		Convey("uncalibrated output cannot establish a strategy label horizon", func() {
			err = predictor.observeResonance(&types.ResonanceArtifact{
				Symbol: "TEST/USD", SupportedHorizon: 9,
				At: time.Unix(1, 0),
			})
			So(err, ShouldBeNil)

			state, stateErr := predictor.state("TEST/USD")
			So(stateErr, ShouldBeNil)
			So(state.horizonSteps, ShouldEqual, 0)
		})

		Convey("calibrated output supplies its exact ticker-step reach", func() {
			err = predictor.observeResonance(&types.ResonanceArtifact{
				Symbol: "TEST/USD", Calibrated: true, SupportedHorizon: 5,
				At: time.Unix(1, 0),
			})
			So(err, ShouldBeNil)

			state, stateErr := predictor.state("TEST/USD")
			So(stateErr, ShouldBeNil)
			So(state.horizonSteps, ShouldEqual, 5)
		})

		Convey("a calibrated artifact without a positive reach fails loudly", func() {
			err = predictor.observeResonance(&types.ResonanceArtifact{
				Symbol: "TEST/USD", Calibrated: true, At: time.Unix(1, 0),
			})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "positive supported horizon")
		})
	})
}

func fullObservationEnvelope() *types.Envelope {
	at := time.Unix(1, 0)
	resonanceSymbol := nmtypes.MustIntern("resonance.metric")
	measurement := data.NewMeasurement[float64](
		"measurement", "TEST/USD", "test", at, time.Time{},
	)
	measurement.Maturity = 1
	measurement.PutMetric(data.Metric[float64]{Label: "raw", Raw: 1})

	envelope := types.NewEnvelope(types.EnvelopeTicker)
	envelope.TickerData.Symbol = "TEST/USD"
	envelope.TickerData.Timestamp = at
	envelope.Correlation = measurement
	envelope.Categories = []types.Category{{
		Symbol: "TEST/USD", Type: types.VerticalIgnition, At: at,
		Confidence: 0.9, Surprisal: 2, Strength: 1, Maturity: 1, Freshness: 1,
	}}
	envelope.Opportunities = []*types.OpportunityCandidate{{
		Symbol: "TEST/USD", Archetype: types.ArchetypeVerticalIgnition,
		Phase: types.PhaseArmed, Direction: types.DirectionLong,
		Provenance: types.ProvenanceCategory, Maturity: 1, Updated: at,
		Economics: &types.OpportunityEconomics{
			Calibrated: true, TransitionProbability: 0.8, ProfitFirst: 0.7, Uncertainty: 0.1,
		},
	}}
	envelope.Cognition = &types.Cognition{
		Symbol: "TEST/USD", At: at, Confidence: 0.8, ClassConfidence: 0.7,
		Contrast: 1, ContrastEvidence: 2, LookaheadScore: 3, InterpolatedSurprisal: 4,
		Classes:       []types.CognitionClass{{Name: "ignition", Probability: 0.8}},
		Contributions: []types.CognitionContribution{{Token: "coil", Bits: 2}},
	}
	envelope.Manifold = &types.ManifoldState{}
	envelope.Resonance = &types.ResonanceArtifact{
		Symbol: "TEST/USD", At: at, Confidence: 0.8,
		Calibrated: true, SupportedHorizon: 3,
	}
	envelope.Resonance.Dynamics.Put(resonanceSymbol, 1)
	envelope.Resonance.Forecast = &types.ResonanceReturnForecast{Call: 1}

	return envelope
}
