package strategy

import (
	"strconv"
	"strings"

	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/symm/system"
)

type CoderReading struct {
	Bullish    bool
	Surprise   float64
	Confidence float64
	PeakCurve  float64
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

	if !ready {
		return CoderReading{Bullish: false, Surprise: surprise}
	}

	// Read forward curve and enforce conviction + cliff drop
	bullish := true
	foundAny := false
	peakCurve := 0.0
	lastVal := 0.0
	cliffDropCount := 0

	maxCliffDrop := 2
	if system.Cfg != nil && system.Cfg.Learning != nil && system.Cfg.Learning.CliffHorizonDrop > 0 {
		maxCliffDrop = system.Cfg.Learning.CliffHorizonDrop
	}

	for h := 0; h < supported; h++ {
		var curveVals []float64
		tick.Lookup(&curveVals, "resonance/"+scope+"/forward_curve_"+strconv.Itoa(h))
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

		// Detect sharp cliff drop: curve collapsing from horizon to horizon
		if h > 0 && val < lastVal*0.5 {
			cliffDropCount++
			if cliffDropCount >= maxCliffDrop {
				bullish = false // Momentum evaporates immediately
				break
			}
		}

		lastVal = val
	}

	// Micro-conviction filter: Must predict at least a minimal move (> 0.15%),
	// not a microscopic drift of 0.00001 on a dead pair.
	if peakCurve < 0.0015 {
		bullish = false
	}

	return CoderReading{
		Bullish:   foundAny && bullish && ready,
		Surprise:  surprise,
		PeakCurve: peakCurve,
	}
}
