package strategy

import (
	"math"
	"strconv"
	"strings"

	"github.com/theapemachine/symm/data"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/system"
)

// ignitionRegimeKey is grid region R07 (Explosive Ignition); entry requires
// this regime when learning.require_ignition_regime is enabled.
const ignitionRegimeKey uint8 = 7

type coderReading struct {
	supportedHorizon float64
	calibrated       float64
	confidence       float64
	surprise         float64
	forward          []float64
}

func resonanceLayerKey(symbol, metric string) string {
	return strings.Join([]string{"resonance", symbolScope(symbol), metric}, "/")
}

func readScalarFromTick(tick *data.Value, symbol, metric string) float64 {
	if tick == nil || symbol == "" {
		return 0
	}

	var values []float64
	tick.Lookup(&values, resonanceLayerKey(symbol, metric))

	if len(values) == 0 {
		return 0
	}

	return values[0]
}

func coderReadingFromTick(tick *data.Value, symbol string) coderReading {
	var out coderReading

	if tick == nil || symbol == "" {
		return out
	}

	readScalar := func(metric string) float64 {
		return readScalarFromTick(tick, symbol, metric)
	}

	out.supportedHorizon = readScalar("supported_horizon")
	out.calibrated = readScalar("calibrated")
	out.confidence = readScalar("confidence")
	out.surprise = readScalar("surprise")

	for horizon := 0; horizon < 16; horizon++ {
		value := readScalar("forward_curve_" + strconv.Itoa(horizon))

		if horizon > 0 && value == 0 && len(out.forward) == 0 {
			break
		}

		if horizon == 0 && value == 0 && readScalar("forward_curve_1") == 0 {
			break
		}

		out.forward = append(out.forward, value)

		if int(out.supportedHorizon) > 0 && horizon+1 >= int(out.supportedHorizon) {
			break
		}
	}

	return out
}

func minSupportedHorizon() int {
	if system.Cfg != nil && system.Cfg.Learning != nil && system.Cfg.Learning.MinSupportedHorizon > 0 {
		return system.Cfg.Learning.MinSupportedHorizon
	}

	return 2
}

func frictionAllowsEntry(regions []*nmstore.Region) (pass bool, checked bool) {
	if system.Cfg == nil || system.Cfg.Learning == nil {
		return true, false
	}

	cfg := system.Cfg.Learning

	if !cfg.RequireFrictionGate {
		return true, false
	}

	min := cfg.FrictionMinBrightness
	if min <= 0 {
		min = 0.35
	}

	for _, region := range regions {
		if region == nil {
			continue
		}

		if region.Key == 12 && region.Brightness >= min {
			return true, true
		}
	}

	return false, true
}

func ignitionLit(regions []*nmstore.Region) bool {
	if system.Cfg == nil || system.Cfg.Learning == nil {
		return true
	}

	cfg := system.Cfg.Learning

	if !cfg.RequireIgnitionRegime {
		return true
	}

	min := cfg.IgnitionMinBrightness
	if min <= 0 {
		min = 0.35
	}

	for _, region := range regions {
		if region == nil {
			continue
		}

		if region.Key == ignitionRegimeKey && region.Brightness >= min {
			return true
		}
	}

	return false
}

func forwardCurveAligned(reading coderReading) bool {
	if len(reading.forward) == 0 {
		return false
	}

	need := minSupportedHorizon()
	if need > len(reading.forward) {
		need = len(reading.forward)
	}

	var sign float64

	for i := 0; i < need; i++ {
		value := reading.forward[i]

		if value == 0 {
			return false
		}

		here := math.Copysign(1, value)

		if sign == 0 {
			sign = here
			continue
		}

		if here != sign {
			return false
		}
	}

	return sign != 0
}

func (model *Model) resonanceAllowsEntry(
	tick *data.Value,
	symbol string,
	regions []*nmstore.Region,
) bool {
	if model == nil || tick == nil {
		return true
	}

	if !ignitionLit(regions) {
		return false
	}

	reading := coderReadingFromTick(tick, symbol)

	if reading.calibrated < 1 {
		return false
	}

	if int(reading.supportedHorizon) < minSupportedHorizon() {
		return false
	}

	if system.Cfg != nil && system.Cfg.Resonance != nil && system.Cfg.Resonance.Gate {
		if reading.confidence > 0 && reading.confidence < 1 {
			return false
		}
	}

	return forwardCurveAligned(reading)
}

