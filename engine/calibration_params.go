package engine

import "time"

/*
CalibrationParams holds tunable thresholds for prediction calibration and confidence normalization.
Injected at construction so analytical models stay independent of global config.
*/
type CalibrationParams struct {
	MinCalibrationSamples int
	MinConfidenceHistory  int
	HalfLifeFloor         time.Duration
	HalfLifeCeiling       time.Duration
	RunwayFactor          float64
	ShockSigma            float64
	RecoveryFactor        float64
	RecoveryBand          float64
	RecoverySamples       int
	BaselineAlpha         float64
}

/*
DefaultCalibrationParams returns paper-trading defaults aligned with config.NewConfig.
*/
func DefaultCalibrationParams() CalibrationParams {
	return CalibrationParams{
		MinCalibrationSamples: 12,
		MinConfidenceHistory:  4,
		HalfLifeFloor:         2 * time.Second,
		HalfLifeCeiling:       15 * time.Minute,
		RunwayFactor:          0.5,
		ShockSigma:            3,
		RecoveryFactor:        6,
		RecoveryBand:          0.1,
		RecoverySamples:       3,
		BaselineAlpha:         0.05,
	}
}

/*
gateParams projects the calibration parameters onto the asymmetric, volatility-gated gain
used by the per-regime scale tracker.
*/
func (params CalibrationParams) gateParams() calibrationGateParams {
	return calibrationGateParams{
		shockSigma:      params.ShockSigma,
		recoveryFactor:  params.RecoveryFactor,
		recoveryBand:    params.RecoveryBand,
		recoverySamples: params.RecoverySamples,
		baselineAlpha:   params.BaselineAlpha,
	}
}

func (params CalibrationParams) minCalibrationSamples() int {
	if params.MinCalibrationSamples <= 0 {
		return 12
	}

	return params.MinCalibrationSamples
}

func (params CalibrationParams) minConfidenceHistory() int {
	if params.MinConfidenceHistory <= 0 {
		return 4
	}

	return params.MinConfidenceHistory
}

func (params CalibrationParams) adaptiveHalfLife(runway time.Duration) time.Duration {
	if runway <= 0 {
		return defaultCalibrationHalfLife
	}

	floor := params.HalfLifeFloor

	if floor <= 0 {
		floor = 2 * time.Second
	}

	ceiling := params.HalfLifeCeiling

	if ceiling <= 0 {
		ceiling = 15 * time.Minute
	}

	halfLife := time.Duration(float64(runway) * params.RunwayFactor)

	if params.RunwayFactor <= 0 {
		halfLife = runway / 2
	}

	if halfLife < floor {
		return floor
	}

	if halfLife > ceiling {
		return ceiling
	}

	return halfLife
}
