package logic

import (
	"math"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

const (
	causalTargetIndex    = 6
	causalTreatmentIndex = 0
	causalHypothesis     = "touch_support_affects_next_l3_epoch_mid_return"
)

var causalControls = []int{1, 2, 3, 4, 5}

type Causal struct {
	symbol  string
	ui      chan []byte
	pearl   *algorithm.Pearl
	pending *causalObservation
	samples uint64
}

type causalObservation struct {
	features []float64
	midPrice float64
	epoch    uint64
	at       time.Time
}

func NewCausal(symbol string, ui chan []byte) *Causal {
	minimumHistory := len(causalControls) + 3

	return &Causal{
		symbol: symbol,
		ui:     ui,
		pearl: algorithm.NewPearl(algorithm.PearlConfig{
			Target:     causalTargetIndex,
			Treatment:  causalTreatmentIndex,
			Controls:   causalControls,
			MinHistory: minimumHistory,
			History:    minimumHistory,
		}),
	}
}

/*
Update aligns one manifold observation with the next observed mid return. The
named treatment and controls are recorded on every output so the result is not
mistaken for a causal claim about anonymous latent columns.
*/
func (causal *Causal) Update(
	state manifold.State,
) (types.Hypothesis, bool, error) {
	if state.At.IsZero() || state.Epoch == 0 {
		return types.Hypothesis{}, false, errnie.Error(errnie.Err(
			errnie.Validation,
			"logic causal: manifold chronology required",
			nil,
		))
	}

	if causal.pending != nil &&
		(state.Epoch <= causal.pending.epoch || !state.At.After(causal.pending.at)) {
		return types.Hypothesis{}, false, errnie.Error(errnie.Err(
			errnie.Validation,
			"logic causal: manifold chronology regressed",
			nil,
		))
	}

	features, ready := causal.features(state)

	if !ready {
		return types.Hypothesis{}, false, nil
	}

	outcome := causal.observe(state, features)
	causal.publish(outcome)

	return types.Hypothesis{
		Source:         types.SourceCausal,
		Symbol:         outcome.Symbol,
		At:             outcome.At,
		Samples:        outcome.Samples,
		Ready:          outcome.Ready,
		Claim:          outcome.Hypothesis,
		Treatment:      outcome.Treatment,
		Controls:       append([]string(nil), outcome.Controls...),
		Outcome:        outcome.Target,
		Association:    outcome.Reading.Association,
		Intervention:   outcome.Reading.Intervention,
		DoExpectation:  outcome.Reading.DoExpectation,
		Uplift:         outcome.Reading.Uplift,
		Counterfactual: outcome.Reading.Counterfactual,
		Confidence:     outcome.Reading.Confidence,
		Strength:       outcome.Reading.Strength,
	}, true, nil
}

func (causal *Causal) observe(
	state manifold.State,
	features []float64,
) CausalOutcome {
	outcome := CausalOutcome{
		Source:     "causal",
		Symbol:     causal.symbol,
		At:         state.At,
		Samples:    causal.samples,
		Hypothesis: causalHypothesis,
		Treatment:  "bid_ask_touch_mass_imbalance",
		Controls: []string{
			"pressure_gradient_x",
			"velocity_divergence",
			"stress_anisotropy",
			"coherence_magnitude_squared",
			"guidance_speed",
		},
		Target: "next_l3_epoch_mid_log_return",
	}

	if causal.pending != nil && state.Epoch > causal.pending.epoch {
		target := math.Log(state.MidPrice / causal.pending.midPrice)
		row := append(append([]float64(nil), causal.pending.features...), target)
		reading, ready, err := causal.pearl.Measure(algorithm.PearlInput{
			Key: causal.symbol,
			Row: row,
		})

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"logic causal: failed to evaluate aligned future outcome",
				err,
			))
		} else {
			causal.samples++
			outcome.Samples = causal.samples
			outcome.Ready = ready
			outcome.Reading = reading
		}
	}

	causal.pending = &causalObservation{
		features: append([]float64(nil), features...),
		midPrice: state.MidPrice,
		epoch:    state.Epoch,
		at:       state.At,
	}

	return outcome
}

func (causal *Causal) features(state manifold.State) ([]float64, bool) {
	touchMass := state.BidTouchDensity + state.AskTouchDensity

	if !state.GasReady() || state.MidPrice <= 0 || touchMass <= 0 {
		return nil, false
	}

	features := []float64{
		(state.BidTouchDensity - state.AskTouchDensity) / touchMass,
		state.PressureGradX,
		state.Divergence,
		state.StressAnisotropy,
		state.CoherenceMag2,
		state.GuidanceSpeed,
	}

	return features, finiteSlice(features)
}

func (causal *Causal) publish(outcome CausalOutcome) {
	if causal.ui == nil {
		return
	}

	select {
	case causal.ui <- datura.Map[any]{"causal": outcome}.Marshal():
	default:
		errnie.Error(errnie.Err(
			errnie.IO,
			"logic causal: UI channel full while publishing outcome",
			nil,
		))
	}
}

/*
CausalOutcome is a named, future-outcome causal hypothesis measurement.
*/
type CausalOutcome struct {
	Source     string                `json:"source"`
	Symbol     string                `json:"symbol"`
	At         time.Time             `json:"at"`
	Samples    uint64                `json:"samples"`
	Ready      bool                  `json:"ready"`
	Hypothesis string                `json:"hypothesis"`
	Treatment  string                `json:"treatment"`
	Controls   []string              `json:"controls"`
	Target     string                `json:"target"`
	Reading    algorithm.PearlOutput `json:"reading"`
}
