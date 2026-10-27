package strategy

import (
	"strconv"
	"strings"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/system"
)

type CoderReading struct {
	Bullish  bool
	Surprise float64
}

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

	bullish := true
	foundAny := false
	horizons := 8

	var supportedVals []float64
	tick.Lookup(&supportedVals, "resonance/"+scope+"/supported_horizon")
	if len(supportedVals) > 0 && supportedVals[0] > 0 {
		horizons = int(supportedVals[0])
	}

	for h := 0; h < horizons; h++ {
		var curveVals []float64
		tick.Lookup(&curveVals, "resonance/"+scope+"/forward_curve_"+strconv.Itoa(h))
		if len(curveVals) == 0 {
			break
		}

		foundAny = true
		if curveVals[0] <= 0 {
			bullish = false
			break
		}
	}

	minHorizon := 2
	if system.Cfg != nil && system.Cfg.Learning != nil && system.Cfg.Learning.MinSupportedHorizon > 0 {
		minHorizon = system.Cfg.Learning.MinSupportedHorizon
	}

	supported := 0
	if len(supportedVals) > 0 {
		supported = int(supportedVals[0])
	}

	ready := false
	var calibratedVals []float64
	tick.Lookup(&calibratedVals, "resonance/"+scope+"/calibrated")
	if len(calibratedVals) > 0 && calibratedVals[0] >= 1 {
		ready = supported >= minHorizon
	}

	return CoderReading{
		Bullish:  foundAny && bullish && ready,
		Surprise: surprise,
	}
}
