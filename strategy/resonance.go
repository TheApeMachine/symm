package strategy

import (
	"strconv"
	"strings"

	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/symm/system"
)

type CoderReading struct {
	Ready      bool
	Bullish    bool
	Surprise   float64
	Confidence float64
	PeakCurve  float64
}

/*
ReadCoder interprets the resonance layers the solver publishes on the tick.
forward_curve_i is the task head's prediction for standardized feature i at the
next epoch, not a price return or a multi-step horizon index.
*/
func ReadCoder(tick *data.Value, symbol string) CoderReading {
	if tick == nil || symbol == "" {
		return CoderReading{}
	}

	scope := strings.ReplaceAll(symbol, "/", "-")

	var surpriseVals []float64
	tick.Lookup(&surpriseVals, "resonance/"+scope+"/surprise")
	surprise := 0.0
	if len(surpriseVals) > 0 {
		surprise = surpriseVals[0]
	}

	var supportedVals []float64
	tick.Lookup(&supportedVals, "resonance/"+scope+"/supported_horizon")
	supported := 0
	if len(supportedVals) > 0 {
		supported = int(supportedVals[0])
	}

	minHorizon := 2
	if system.Cfg != nil && system.Cfg.Learning != nil && system.Cfg.Learning.MinSupportedHorizon > 0 {
		minHorizon = system.Cfg.Learning.MinSupportedHorizon
	}

	ready := false
	var calibratedVals []float64
	tick.Lookup(&calibratedVals, "resonance/"+scope+"/calibrated")
	if len(calibratedVals) > 0 && calibratedVals[0] >= 1 {
		ready = supported >= minHorizon
	}

	confidence := 0.0
	var precisionVals []float64
	tick.Lookup(&precisionVals, "resonance/"+scope+"/task_relative_precision")
	if len(precisionVals) > 0 {
		confidence = precisionVals[0]
	}

	if !ready {
		return CoderReading{Ready: false, Bullish: false, Surprise: surprise, Confidence: confidence}
	}

	// Require cooperative positive shifts across the predicted feature vector.
	bullish := true
	foundAny := false
	peakCurve := 0.0

	for i := 0; i < supported; i++ {
		var curveVals []float64
		tick.Lookup(&curveVals, "resonance/"+scope+"/forward_curve_"+strconv.Itoa(i))
		if len(curveVals) == 0 {
			break
		}

		val := curveVals[0]
		foundAny = true

		if val <= 0 {
			bullish = false
			break
		}

		if val > peakCurve {
			peakCurve = val
		}
	}

	// Minimal conviction in standardized feature space (z-scores), not price %.
	const minFeatureZ = 0.15
	if peakCurve < minFeatureZ {
		bullish = false
	}

	return CoderReading{
		Ready:      ready,
		Bullish:    foundAny && bullish && ready,
		Surprise:   surprise,
		Confidence: confidence,
		PeakCurve:  peakCurve,
	}
}
