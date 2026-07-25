package logic

import (
	"math"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

const (
	causalTargetIndex    = 6
	causalTreatmentIndex = 0
	causalHypothesis     = "trade_arrival_imbalance_affects_next_l3_epoch_mid_return"
)

var causalControls = []int{1, 2, 3, 4, 5}

/*
Causal aligns physical observations with the next realized midpoint return and
retains online Pearl evidence for one symbol. Return-error calibration belongs
to the strict-prior Resonance RLS head; Pearl evaluates a resolved causal row.
*/
type Causal struct {
	symbol  string
	pearl   *algorithm.Pearl
	pending *causalObservation
	last    *CausalOutcome
	samples uint64
}

/*
causalObservation retains the treatment and control row awaiting the next
manifold epoch, where its realized return completes the causal sample.
*/
type causalObservation struct {
	features []float64
	midPrice *decimal.Decimal
	epoch    uint64
	at       time.Time
}

/*
NewCausal creates the symbol-local Pearl model whose outcomes are returned to
Analyzer for accumulation on the current Thesis alongside durable Hypotheses.
*/
func NewCausal(symbol string) *Causal {
	minimumHistory := len(causalControls) + 3

	return &Causal{
		symbol: symbol,
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
Update aligns one manifold observation with the next observed mid return and
returns both its durable Hypothesis and causal outcome for the current Thesis.
*/
func (causal *Causal) Update(
	state manifold.State,
) (types.Hypothesis, *CausalOutcome, error) {
	if state.At.IsZero() || state.Epoch == 0 {
		return types.Hypothesis{}, nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"logic causal: manifold chronology required",
			nil,
		))
	}

	if causal.pending != nil && state.Epoch == causal.pending.epoch &&
		!state.At.After(causal.pending.at) {
		// Exact same timestamp re-presented: nothing new to align.
		return types.Hypothesis{}, nil, nil
	}

	if causal.pending != nil &&
		(state.Epoch < causal.pending.epoch || state.At.Before(causal.pending.at)) {
		if state.Epoch != 1 {
			return types.Hypothesis{}, nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic causal: manifold chronology regressed without re-admission",
				nil,
			))
		}

		// A symbol can leave the shared observed universe and later return at a
		// fresh epoch 1; that new admission cannot resolve the orphaned prior row.
		causal.pending = nil
	}

	features, ready := causal.features(state)

	if !ready {
		return types.Hypothesis{}, nil, nil
	}

	if causal.pending != nil && state.Epoch == causal.pending.epoch {
		// Always-step on the same Hawkes epoch: refresh the pending row and
		// republish the last ready reading so ResetTick cannot strand forecasts.
		causal.pending.features = append(causal.pending.features[:0], features...)
		causal.pending.midPrice = state.ReferencePrice.Copy()
		causal.pending.at = state.At

		if causal.last == nil {
			return types.Hypothesis{}, nil, nil
		}

		outcome := *causal.last
		outcome.At = state.At

		return causal.hypothesis(outcome), &outcome, nil
	}

	outcome, err := causal.observe(state, features)

	if err != nil {
		return types.Hypothesis{}, nil, err
	}

	if outcome.Ready {
		cloned := outcome
		causal.last = &cloned
	}

	return causal.hypothesis(outcome), &outcome, nil
}

/*
hypothesis projects one causal outcome onto the durable Thesis hypothesis row.
*/
func (causal *Causal) hypothesis(outcome CausalOutcome) types.Hypothesis {
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
	}
}

func (causal *Causal) observe(
	state manifold.State,
	features []float64,
) (CausalOutcome, error) {
	outcome := CausalOutcome{
		Source:     "causal",
		Symbol:     causal.symbol,
		At:         state.At,
		Samples:    causal.samples,
		Hypothesis: causalHypothesis,
		Treatment:  "buy_sell_arrival_intensity_imbalance",
		Controls: []string{
			"pressure_gradient_x",
			"velocity_divergence",
			"stress_anisotropy",
			"coherence_magnitude_squared",
			"guidance_speed",
		},
		Target: "next_l3_epoch_mid_log_return",
	}

	if causal.pending != nil && state.Epoch == causal.pending.epoch+1 {
		outcome.At = causal.pending.at

		if causal.pending.midPrice == nil || causal.pending.midPrice.Sign() <= 0 ||
			state.ReferencePrice == nil ||
			state.ReferencePrice.Sign() <= 0 {
			return CausalOutcome{}, errnie.Err(
				errnie.Validation,
				"logic causal: midpoint prices must be strictly positive for log return",
				nil,
			)
		}

		scale := max(
			int64(decimal.DefaultScale),
			state.ReferencePrice.GetScale(),
			causal.pending.midPrice.GetScale(),
		)
		ratio := state.ReferencePrice.SetScale(scale).
			Div(causal.pending.midPrice.SetScale(scale)).
			Float64()

		if !(ratio > 0) {
			return CausalOutcome{}, errnie.Err(
				errnie.Validation,
				"logic causal: log-return argument must be strictly positive",
				nil,
			)
		}

		target := math.Log(ratio)

		row := append(append([]float64(nil), causal.pending.features...), target)
		reading, ready, err := causal.pearl.Measure(algorithm.PearlInput{
			Key: causal.symbol,
			Row: row,
		})

		if err != nil {
			return CausalOutcome{}, errnie.Err(
				errnie.UnprocessableContent,
				"logic causal: failed to evaluate aligned future outcome",
				err,
			)
		}

		causal.samples++
		outcome.Samples = causal.samples
		outcome.Ready = ready
		outcome.Reading = reading
	}

	causal.pending = &causalObservation{
		features: append([]float64(nil), features...),
		midPrice: state.ReferencePrice.Copy(),
		epoch:    state.Epoch,
		at:       state.At,
	}

	if outcome.Ready && finite(outcome.Reading.DoExpectation) {
		outcome.ExpectedReturn = outcome.Reading.DoExpectation
	}

	if outcome.Ready && finite(outcome.Reading.Confidence) {
		// Flow is informed when a causal link from arrival imbalance to the
		// next return is established AND the current arrivals are one-sided:
		// the model's category-share confidence times the present imbalance
		// magnitude, both in [0,1], so the product is a joint probability
		// with no free constants.
		outcome.InformedFlow = math.Abs(features[causalTreatmentIndex]) *
			math.Max(outcome.Reading.Confidence, 0)
	}

	return outcome, nil
}

func (causal *Causal) features(state manifold.State) ([]float64, bool) {
	if !state.GasReady() || state.ReferencePrice == nil ||
		state.ReferencePrice.Sign() <= 0 {
		return nil, false
	}

	arrivalMass := state.BuyIntensity + state.SellIntensity

	if arrivalMass <= 0 {
		return nil, false
	}

	features := []float64{
		(state.BuyIntensity - state.SellIntensity) / arrivalMass,
		state.Reading.PressureGradX,
		state.Reading.Divergence,
		state.StressAnisotropy,
		state.Reading.CoherenceMag2,
		state.Reading.GuidanceSpeed,
	}

	return features, finiteSlice(features)
}

/*
CausalOutcome is a named, future-outcome causal hypothesis measurement.
InformedFlow is the probability the current arrival flow is informed — the
established strength of the imbalance→return causal link scaled by how
one-sided the present arrivals are — which the analyzer prices as adverse
selection against the observed touch cost.
*/
type CausalOutcome struct {
	Source         string                `json:"source"`
	Symbol         string                `json:"symbol"`
	At             time.Time             `json:"at"`
	Samples        uint64                `json:"samples"`
	Ready          bool                  `json:"ready"`
	Hypothesis     string                `json:"hypothesis"`
	Treatment      string                `json:"treatment"`
	Controls       []string              `json:"controls"`
	Target         string                `json:"target"`
	Reading        algorithm.PearlOutput `json:"reading"`
	ExpectedReturn float64               `json:"expectedReturn"`
	InformedFlow   float64               `json:"informedFlow"`
}
