package manifold

import (
	"fmt"
	"math"

	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

const (
	forecastFeatureCount = 6
	forecastModelVersion = "rls-next-l3-v1"
	forecastTarget       = "next_l3_epoch_mid_log_return"
)

/*
ForecastConfig contains the explicit RLS prior and memory policy.
*/
type ForecastConfig struct {
	InitialVariance  float64
	ForgettingFactor float64
}

/*
ForecastModel learns the next L3-epoch mid return from prior manifold states.
It scores every forecast before learning its realized target.
*/
type ForecastModel struct {
	learner     *learning.RLS
	calibration *Calibration
	pending     *forecastObservation
}

type forecastObservation struct {
	features  []float64
	predicted float64
	midPrice  float64
	epoch     uint64
}

func NewForecastModel(config ForecastConfig) (*ForecastModel, error) {
	learner, err := learning.NewRLS(learning.RLSConfig{
		Dimension:        forecastFeatureCount,
		InitialVariance:  config.InitialVariance,
		ForgettingFactor: config.ForgettingFactor,
	})

	if err != nil {
		return nil, fmt.Errorf("logic manifold forecast: %w", err)
	}

	return &ForecastModel{
		learner:     learner,
		calibration: &Calibration{},
	}, nil
}

/*
Update observes the realized target for the prior state, then emits a forecast
for the current state. The returned forecast is immediately usable by paper
strategy; calibration remains an observable property rather than a blocker.
*/
func (model *ForecastModel) Update(state State) (types.Forecasts, bool, error) {
	features, ready := model.features(state)

	if !ready {
		return types.Forecasts{}, false, nil
	}

	if model.pending != nil && state.Epoch > model.pending.epoch {
		actual := math.Log(state.MidPrice / model.pending.midPrice)
		model.calibration.Observe(model.pending.predicted, actual)

		if _, err := model.learner.Measure(learning.RLSSample{
			Features: model.pending.features,
			Target:   actual,
		}); err != nil {
			return types.Forecasts{}, false, err
		}
	}

	prediction, err := model.learner.Predict(features)

	if err != nil {
		return types.Forecasts{}, false, err
	}

	calibration := model.calibration.Snapshot(forecastFeatureCount)
	forecast := types.Forecasts{
		Source:                   "manifold_forecast",
		Symbol:                   state.Symbol,
		At:                       state.At,
		ObservedInterval:         state.Duration(),
		SourceEpoch:              state.Epoch,
		HorizonEvents:            1,
		ExpiresEpoch:             state.Epoch + 1,
		Target:                   forecastTarget,
		ModelVersion:             forecastModelVersion,
		Ready:                    true,
		Calibrated:               calibration.Calibrated,
		FrictionReady:            state.HasSpread(),
		CalibrationSamples:       calibration.Samples,
		IncrementalMSE:           calibration.IncrementalMSE,
		IncrementalMSELowerBound: calibration.LowerBound,
		ExpectedReturn:           prediction.Value,
		ReferencePrice:           state.MidPrice,
		BuyCapacity:              state.BestAsk * state.BestAskQuantity,
		SellCapacity:             state.BestBid * state.BestBidQuantity,
		ExpectedSpread:           state.SpreadReturn(),
		Uncertainty:              calibration.ResidualStdDev,
		Confidence:               model.calibration.Confidence(prediction.Value),
	}

	model.pending = &forecastObservation{
		features:  append([]float64(nil), features...),
		predicted: prediction.Value,
		midPrice:  state.MidPrice,
		epoch:     state.Epoch,
	}

	return forecast, true, nil
}

func (model *ForecastModel) features(state State) ([]float64, bool) {
	if !state.GasReady() || state.MidPrice <= 0 {
		return nil, false
	}

	touchMass := state.BidTouchDensity + state.AskTouchDensity

	if touchMass <= 0 {
		return nil, false
	}

	features := []float64{
		(state.BidTouchDensity - state.AskTouchDensity) / touchMass,
		state.PressureGradX,
		state.Divergence,
		state.CoherenceMag2,
		state.GuidanceSpeed,
		state.StressAnisotropy,
	}

	for _, value := range features {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, false
		}
	}

	return features, true
}
