package logic

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

const (
	causalTargetIndex    = 6
	causalTreatmentIndex = 0
	causalHypothesis     = "touch_support_affects_next_l3_epoch_mid_return"
)

var causalControls = []int{1, 2, 3, 4, 5}

/*
Causal aligns physical observations with the next realized midpoint return and
retains online Pearl evidence plus forecast-error calibration for one symbol.
*/
type Causal struct {
	symbol  string
	pearl   *algorithm.Pearl
	pending *causalObservation
	mse     *statistic.Mean
	errors  *adaptive.Variance
	samples uint64
}

/*
causalObservation retains the feature row and prediction awaiting the next
manifold epoch, where the realized return can calibrate that prediction.
*/
type causalObservation struct {
	features        []float64
	midPrice        float64
	epoch           uint64
	at              time.Time
	predictedReturn float64
	predicted       bool
}

/*
NewCausal creates the symbol-local Pearl model whose outcomes are returned to
Analyzer for accumulation on the current Thesis alongside durable Hypotheses.
*/
func NewCausal(symbol string) *Causal {
	minimumHistory := len(causalControls) + 3

	return &Causal{
		symbol: symbol,
		mse:    statistic.NewMean(),
		errors: adaptive.NewVariance(),
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
		!state.At.Before(causal.pending.at) {
		// Same epoch re-presented (idle symbol republished with its last
		// GasReady state): nothing new to align, skip without erroring.
		return types.Hypothesis{}, nil, nil
	}

	if causal.pending != nil &&
		(state.Epoch < causal.pending.epoch || state.At.Before(causal.pending.at)) {
		return types.Hypothesis{}, nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"logic causal: manifold chronology regressed",
			nil,
		))
	}

	features, ready := causal.features(state)

	if !ready {
		return types.Hypothesis{}, nil, nil
	}

	outcome, err := causal.observe(state, features)

	if err != nil {
		return types.Hypothesis{}, nil, err
	}

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
	}, &outcome, nil
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

	if causal.pending != nil && state.Epoch == causal.pending.epoch+1 {
		if !(causal.pending.midPrice > 0) || !(state.ReferencePrice > 0) {
			return CausalOutcome{}, errnie.Err(
				errnie.Validation,
				"logic causal: midpoint prices must be strictly positive for log return",
				nil,
			)
		}

		ratio := state.ReferencePrice / causal.pending.midPrice

		if !(ratio > 0) {
			return CausalOutcome{}, errnie.Err(
				errnie.Validation,
				"logic causal: log-return argument must be strictly positive",
				nil,
			)
		}

		target := math.Log(ratio)

		if causal.pending.predicted {
			residual := target - causal.pending.predictedReturn
			mse, err := causal.mse.Measure(residual * residual)

			if err != nil {
				return CausalOutcome{}, errnie.Err(
					errnie.UnprocessableContent,
					"logic causal: failed to calibrate forecast error",
					err,
				)
			}

			errorVariance, err := causal.errors.Measure(residual)

			if err != nil {
				return CausalOutcome{}, errnie.Err(
					errnie.UnprocessableContent,
					"logic causal: failed to measure forecast uncertainty",
					err,
				)
			}

			outcome.CalibrationSamples = uint64(mse.Count)
			outcome.IncrementalMSE = mse.Value
			outcome.Uncertainty = math.Sqrt(math.Max(errorVariance.Value, 0))
		}

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
		midPrice: state.ReferencePrice,
		epoch:    state.Epoch,
		at:       state.At,
	}

	if outcome.Ready && finite(outcome.Reading.DoExpectation) {
		causal.pending.predictedReturn = outcome.Reading.DoExpectation
		causal.pending.predicted = true
		outcome.ExpectedReturn = outcome.Reading.DoExpectation
	}

	return outcome, nil
}

func (causal *Causal) features(state manifold.State) ([]float64, bool) {
	if !state.GasReady() || state.ReferencePrice <= 0 {
		return nil, false
	}

	touchMass := state.BuyIntensity + state.SellIntensity

	if touchMass <= 0 {
		return nil, false
	}

	features := []float64{
		(state.BuyIntensity - state.SellIntensity) / touchMass,
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
*/
type CausalOutcome struct {
	Source             string                `json:"source"`
	Symbol             string                `json:"symbol"`
	At                 time.Time             `json:"at"`
	Samples            uint64                `json:"samples"`
	Ready              bool                  `json:"ready"`
	Hypothesis         string                `json:"hypothesis"`
	Treatment          string                `json:"treatment"`
	Controls           []string              `json:"controls"`
	Target             string                `json:"target"`
	Reading            algorithm.PearlOutput `json:"reading"`
	ExpectedReturn     float64               `json:"expectedReturn"`
	IncrementalMSE     float64               `json:"incrementalMSE"`
	Uncertainty        float64               `json:"uncertainty"`
	CalibrationSamples uint64                `json:"calibrationSamples"`
}
