package strategy

import (
	"strconv"
	"strings"

	"github.com/theapemachine/symm/data"
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

	for h := 0; h < 8; h++ {
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

	return CoderReading{
		Bullish:  foundAny && bullish,
		Surprise: surprise,
	}
}
