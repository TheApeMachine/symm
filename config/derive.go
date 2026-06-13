package config

import (
	"fmt"
	"math"
	"time"

	"github.com/spf13/viper"
)

const numericGuard = 1e-12

/*
RegimeSpec holds derived regime window sizing from operational inputs only.
*/
type RegimeSpec struct {
	Window     int
	MinSamples int
}

/*
BaselineSpec holds EWMA adaptation parameters derived from the regime window.
*/
type BaselineSpec struct {
	AlphaMin         float64
	AlphaMax         float64
	MinObs           int
	TrendSigma       float64
	StrongTrendSigma float64
	VolFloorSigma    float64
	VolScaleFloor    float64
}

/*
CrossSectionSpec holds derived cross-section capacities from regime sizing.
*/
type CrossSectionSpec struct {
	MatchWindow time.Duration
	ReturnCap   int
	MinBars     int
	BreadthHist int
}

/*
DerivedRegimeSpec computes regime window and warmup counts from gauge cadence
and the subscribed symbol universe — not from static trading constants.
*/
func DerivedRegimeSpec() (RegimeSpec, error) {
	publishInterval := viper.GetDuration("telemetry.gauge.publish_interval")

	if publishInterval <= 0 {
		publishInterval = 100 * time.Millisecond
	}

	symbolCount := max(len(viper.GetStringSlice("market.default_symbols")), 1)

	ticksPerQuarterHour := int((15 * time.Minute) / publishInterval)
	universeScale := 1 + int(math.Log2(float64(symbolCount)))
	window := ticksPerQuarterHour * universeScale
	window = max(nextPowerOfTwo(window), 64)

	minSamples := int(math.Max(4, math.Round(math.Log2(float64(window)))))

	return RegimeSpec{
		Window:     window,
		MinSamples: minSamples,
	}, nil
}

/*
DerivedBaselineSpec turns the regime window into EWMA and sigma parameters.
*/
func DerivedBaselineSpec(regime RegimeSpec) BaselineSpec {
	window := float64(regime.Window)
	minObs := regime.MinSamples

	alphaMin := 2.0 / window
	alphaMax := math.Min(0.25, 32.0/window)

	if alphaMax <= alphaMin {
		alphaMax = alphaMin * 4
	}

	trendSigma := 1.0 + math.Log2(window/64.0)

	if trendSigma < 1.0 {
		trendSigma = 1.0
	}

	strongTrendSigma := trendSigma * 2.0
	volFloorSigma := strongTrendSigma + trendSigma*0.2

	return BaselineSpec{
		AlphaMin:         alphaMin,
		AlphaMax:         alphaMax,
		MinObs:           minObs,
		TrendSigma:       trendSigma,
		StrongTrendSigma: strongTrendSigma,
		VolFloorSigma:    volFloorSigma,
		VolScaleFloor:    numericGuard,
	}
}

/*
DerivedCrossSectionSpec sizes cross-section buffers from regime window and gauge cadence.
*/
func DerivedCrossSectionSpec(regime RegimeSpec) CrossSectionSpec {
	publishInterval := viper.GetDuration("telemetry.gauge.publish_interval")

	if publishInterval <= 0 {
		publishInterval = 100 * time.Millisecond
	}

	matchWindow := 60 * publishInterval

	if matchWindow < time.Second {
		matchWindow = time.Second
	}

	capacity := regime.Window / 4

	if capacity < regime.MinSamples {
		capacity = regime.MinSamples
	}

	minBars := int(math.Max(float64(regime.MinSamples), math.Sqrt(float64(regime.Window))))

	return CrossSectionSpec{
		MatchWindow: matchWindow,
		ReturnCap:   capacity,
		MinBars:     minBars,
		BreadthHist: capacity,
	}
}

/*
DerivedPublishInterval returns the parent gauge cadence used to scale child timers.
*/
func DerivedPublishInterval() time.Duration {
	interval := viper.GetDuration("telemetry.gauge.publish_interval")

	if interval <= 0 {
		return 100 * time.Millisecond
	}

	return interval
}

/*
DerivedManifoldLattice sizes the 3D solver grid from book depth and universe breadth.
*/
func DerivedManifoldLattice(bookDepth, symbolCount int) (
	gridX, gridY, gridZ uint32, halfWidth int,
) {
	if bookDepth < 1 {
		bookDepth = 1
	}

	if symbolCount < 1 {
		symbolCount = 1
	}

	gridX = uint32(bookDepth * 4)
	gridY = max(uint32(symbolCount), 3)

	gridZ = uint32(nextPowerOfTwo(symbolCount * 2))
	halfWidth = bookDepth * 3

	gridX = max(gridX, 3)
	gridY = max(gridY, 3)
	gridZ = max(gridZ, 3)

	return gridX, gridY, gridZ, halfWidth
}

/*
DerivedSolverTickSize estimates lattice price increment from subscribed book depth.
*/
func DerivedSolverTickSize(bookDepth int) float64 {
	if bookDepth < 1 {
		bookDepth = 1
	}

	latticeScale := nextPowerOfTwo(bookDepth)

	return 1.0 / float64(latticeScale*latticeScale)
}

/*
DerivedPredictionHorizon aligns forecast maturity with the quarter-hour gauge span.
*/
func DerivedPredictionHorizon() time.Duration {
	publishInterval := DerivedPublishInterval()
	ticksPerQuarterHour := (15 * time.Minute) / publishInterval

	if ticksPerQuarterHour <= 0 {
		return 15 * time.Minute
	}

	return publishInterval * ticksPerQuarterHour
}

func NumericGuard() float64 {
	return numericGuard
}

/*
DerivedVolumeClockBarsPerDay estimates how many gauge frames land in one day.
*/
func DerivedVolumeClockBarsPerDay() float64 {
	interval := DerivedPublishInterval()

	return float64(24 * time.Hour / interval)
}

/*
DerivedBookDepthLevels reads structural book depth from market config when set,
otherwise derives one level per default symbol.
*/
func DerivedBookDepthLevels() (int, error) {
	depth := viper.GetInt("market.book_depth_levels")

	if depth > 0 {
		return depth, nil
	}

	symbolCount := len(viper.GetStringSlice("market.default_symbols"))

	if symbolCount < 1 {
		return 0, fmt.Errorf("config: market.book_depth_levels or default_symbols required")
	}

	return symbolCount, nil
}

func nextPowerOfTwo(value int) int {
	if value <= 1 {
		return 1
	}

	power := 1

	for power < value {
		power <<= 1
	}

	return power
}
