package strategy

import (
	"strconv"
	"strings"

	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/symm/logic/resonance"
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
forward_curve_i is the task head's prediction for standardized Features[i] at
the next epoch (see logic/resonance/features.go), not a price return.
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

	curve := readForwardCurve(tick, scope, supported)
	enterNet, holdNet, exitNet, risk, ok := resonance.ForwardStoryNets(curve)
	if !ok {
		return CoderReading{Ready: ready, Bullish: false, Surprise: surprise, Confidence: confidence}
	}

	peakCurve := enterNet
	if holdNet > peakCurve {
		peakCurve = holdNet
	}

	const minFeatureZ = 0.15
	bullish := enterNet >= minFeatureZ &&
		holdNet >= minFeatureZ &&
		exitNet <= holdNet &&
		risk <= enterNet

	return CoderReading{
		Ready:      ready,
		Bullish:    bullish,
		Surprise:   surprise,
		Confidence: confidence,
		PeakCurve:  peakCurve,
	}
}

func readForwardCurve(tick *data.Value, scope string, supported int) []float64 {
	if supported <= 0 {
		return nil
	}

	curve := make([]float64, 0, supported)
	for i := 0; i < supported; i++ {
		var vals []float64
		tick.Lookup(&vals, "resonance/"+scope+"/forward_curve_"+strconv.Itoa(i))
		if len(vals) == 0 {
			break
		}
		curve = append(curve, vals[0])
	}

	return curve
}
