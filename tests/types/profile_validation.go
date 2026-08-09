package types

import (
	"fmt"
	"math"
)

func validateProfiles(
	profiles map[MarketState]RegimeProfile,
	momentum map[MarketState]float64,
) error {
	if len(profiles) == 0 {
		return fmt.Errorf("scenario: regime profiles are required")
	}

	baseline, known := profiles[Baseline]

	if !known {
		return fmt.Errorf("scenario: baseline regime profile is required")
	}

	if baseline.Precursor.MinimumObservations < 1 {
		return fmt.Errorf("scenario: baseline regime requires positive precursor observations")
	}

	for state, profile := range profiles {
		if err := validateProfile(state, profile); err != nil {
			return err
		}

		speed, known := momentum[state]

		if !known || math.IsNaN(speed) || math.IsInf(speed, 0) {
			return fmt.Errorf("scenario: regime %d requires finite transition momentum", state)
		}
	}

	return nil
}

func validateProfile(state MarketState, profile RegimeProfile) error {
	if profile.Cadence <= 0 || profile.SpreadScale <= 0 ||
		profile.BidAskAsymmetry <= 0 || profile.BaseQty <= 0 ||
		profile.VolumeScale <= 0 {
		return fmt.Errorf("scenario: regime %d cadence, spread, depth, and volume must be positive", state)
	}

	values := []float64{
		profile.Drift, profile.Diffusion, profile.Volatility,
		profile.SpreadScale, profile.SpreadJitter, profile.MeanReversion,
		profile.OscillationMove, profile.BidAskAsymmetry, profile.BaseQty,
		profile.VolumeScale, profile.IgnitionMove, profile.IgnitionVolume,
		profile.IgnitionDecay,
	}

	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("scenario: regime %d values must be finite", state)
		}
	}

	if profile.Diffusion < 0 || profile.Volatility < 0 ||
		profile.SpreadJitter < 0 || profile.MeanReversion < 0 ||
		profile.IgnitionVolume < 0 || profile.IgnitionDecay < 0 ||
		profile.IgnitionDecay > 1 || profile.Precursor.MinimumObservations < 0 {
		return fmt.Errorf("scenario: regime %d dispersion, ignition, and precursor values are invalid", state)
	}

	if profile.IgnitionMove != 0 && profile.IgnitionDecay >= 1 {
		return fmt.Errorf("scenario: regime %d ignition must decay to completion", state)
	}

	if profile.AggressorSide != "" && profile.AggressorSide != "buy" &&
		profile.AggressorSide != "sell" {
		return fmt.Errorf("scenario: regime %d aggressor side is invalid", state)
	}

	return nil
}
