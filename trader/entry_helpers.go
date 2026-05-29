package trader

import (
	"strings"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

/*
sourceContributions returns the per-source max confidence that fed the fusion
behind this entry.
*/
func sourceContributions(measurements []engine.Measurement) map[string]float64 {
	contributions := make(map[string]float64, len(measurements))

	for _, measurement := range measurements {
		if measurement.Confidence <= 0 || measurement.Source == "" {
			continue
		}

		if measurement.Confidence > contributions[measurement.Source] {
			contributions[measurement.Source] = measurement.Confidence
		}
	}

	return contributions
}

/*
dominantSource returns the strongest source among the fused measurements.
*/
func dominantSource(measurements []engine.Measurement) string {
	best := ""
	bestConfidence := 0.0

	for _, measurement := range measurements {
		if measurement.Source == "" {
			continue
		}

		if measurement.Confidence > bestConfidence {
			bestConfidence = measurement.Confidence
			best = measurement.Source
		}
	}

	return best
}

func symbolBase(symbol string) string {
	base, _, _ := strings.Cut(symbol, "/")

	return base
}

func perspectiveType(measurement engine.Measurement) engine.PerspectiveType {
	switch measurement.Type {
	case engine.LeadLag:
		return engine.PerspectiveCrossAsset
	case engine.Sentiment:
		return engine.PerspectiveSentiment
	case engine.Flow, engine.DepthFlow:
		return engine.PerspectiveFlow
	default:
		return engine.PerspectiveMicrostructure
	}
}

func runwayForPerspective(perspective engine.Perspective) time.Duration {
	runway := time.Duration(0)

	for _, measurement := range perspective.Measurements {
		if measurement.Timeframe.End <= measurement.Timeframe.Start {
			continue
		}

		candidate := time.Duration(measurement.Timeframe.End-measurement.Timeframe.Start) * time.Second

		if candidate > runway {
			runway = candidate
		}
	}

	if runway > 0 {
		return runway
	}

	for _, measurement := range perspective.Measurements {
		switch measurement.Type {
		case engine.Flow, engine.DepthFlow:
			return config.System.FlowHoldBeforeExit
		case engine.Causal:
			return config.System.MinHoldBeforeRotate
		}
	}

	return config.System.ScalpHoldBeforeExit
}

func predictionDirection(perspective engine.Perspective) int {
	score := 0.0

	for _, measurement := range perspective.Measurements {
		score += measurement.Confidence * float64(measurementDirection(measurement))
	}

	if score < 0 {
		return -1
	}

	return 1
}

func measurementDirection(measurement engine.Measurement) int {
	switch measurement.Type {
	case engine.Dump:
		return -1
	default:
		return 1
	}
}

/*
pumpRegimeOf classifies a pump measurement into a fast or slow regime.
*/
func pumpRegimeOf(measurement engine.Measurement) string {
	if measurement.Source != "pumpdump" {
		return ""
	}

	switch measurement.Reason {
	case "fast_pump":
		return "pump_fast"
	case "actual_pump", "slow_breakout":
		return "pump_slow"
	}

	return ""
}

func isPumpRegime(regime string) bool {
	return regime == "pump_fast" || regime == "pump_slow"
}
