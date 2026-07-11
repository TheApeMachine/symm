package manifold

import (
	"math"
	"time"

	"github.com/theapemachine/symm/strategy"
)

/*
Forecasts holds calibrated observable predictions derived from typed physical readout.
Strategy consumes these instead of raw projection labels.
*/
type Forecasts struct {
	At               time.Time
	BidTouchSurvival float64
	AskTouchSurvival float64
	TimeToDepletion  float64
	SpreadNarrowing  float64
	MidMove          float64
	ExecutableReturn float64
	Replenishment    float64
	ImpactEstimate   float64
	Uncertainty      float64
}

/*
Forecaster derives event-time forecasts from State without category scores or
fixed horizons.
*/
type Forecaster struct{}

func NewForecaster() *Forecaster {
	return &Forecaster{}
}

func (forecaster *Forecaster) Forecast(state State) Forecasts {
	if !state.GasReady() {
		return Forecasts{At: state.At}
	}

	touchMass := state.BidTouchDensity + state.AskTouchDensity

	if touchMass <= 0 {
		return Forecasts{At: state.At}
	}

	bidShare := state.BidTouchDensity / touchMass
	askShare := state.AskTouchDensity / touchMass
	coherence := state.CoherenceMag2
	divergence := math.Abs(state.Divergence)
	viscosity := math.Max(state.ViscosityProxy, 0)
	stress := math.Max(state.StressAnisotropy, 0)
	guidance := state.GuidanceSpeed
	pressureDrive := state.PressureGradNorm

	supportRetention := coherence / (1 + divergence + viscosity)
	bidSurvival := bidShare * supportRetention
	askSurvival := askShare * supportRetention

	depletionRate := divergence + stress
	timeToDepletion := 0.0

	if depletionRate > 0 {
		timeToDepletion = touchMass / depletionRate
	}

	spreadNarrowing := (bidShare - askShare) * (1 - stress)
	midMove := guidance

	if pressureDrive > 0 {
		midMove += math.Copysign(pressureDrive, state.Divergence)
	}

	impactEstimate := stress * touchMass
	executableReturn := midMove - impactEstimate
	replenishment := supportRetention * (1 - stress)
	uncertainty := (divergence + stress + viscosity) / (1 + coherence)

	return Forecasts{
		At:               state.At,
		BidTouchSurvival: bidSurvival,
		AskTouchSurvival: askSurvival,
		TimeToDepletion:  timeToDepletion,
		SpreadNarrowing:  spreadNarrowing,
		MidMove:          midMove,
		ExecutableReturn: executableReturn,
		Replenishment:    replenishment,
		ImpactEstimate:   impactEstimate,
		Uncertainty:      uncertainty,
	}
}

func (forecaster *Forecaster) Attach(thesis *strategy.Thesis, state State) *strategy.Thesis {
	forecasts := forecaster.Forecast(state)
	thesis.AddEvidence("manifold_forecasts", forecasts)

	return thesis
}
