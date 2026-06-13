package liquidity

import (
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/logic"
	"gonum.org/v1/gonum/stat"
)

func liquidityBaselineWindow() time.Duration {
	baselineWindow := viper.GetDuration("signals.liquidity.baseline_window")

	if baselineWindow > 0 {
		return baselineWindow
	}

	matchWindow := viper.GetDuration("signals.trade_match_window")

	if matchWindow > 0 {
		return matchWindow
	}

	return time.Minute
}

func (signal *Signal) observeVolumeBaseline(
	quoteVol float64,
	at time.Time,
) (float64, bool, error) {
	if signal.volumeBaseline == nil {
		return 0, false, nil
	}

	ready := signal.volumeBaseline.Initialized()
	relative, err := signal.volumeBaseline.Update(at, quoteVol)

	if err != nil {
		return 0, false, err
	}

	return relative, ready, nil
}

func (signal *Signal) historicallyLiquid(
	relativeVolume float64,
	ready bool,
	peers []float64,
	quoteVol float64,
) bool {
	if !ready || quoteVol <= 0 || len(peers) < 2 {
		return false
	}

	median := float64(statistic.NewMedian(nil).Observe(nomagique.Numbers(peers...)...))

	if median <= 0 {
		return false
	}

	peerRelatives := make([]float64, len(peers))

	for index, peerVolume := range peers {
		peerRelatives[index] = peerVolume / median
	}

	liquidThreshold := float64(
		statistic.NewQuantile(0.75, stat.LinInterp, nil).Observe(nomagique.Numbers(peerRelatives...)...),
	)

	return relativeVolume >= liquidThreshold
}

func (signal *Signal) quartiles(volumes []float64) (lower, upper float64) {
	return float64(signal.quantile25.Observe(nomagique.Numbers(volumes...)...)),
		float64(signal.quantile75.Observe(nomagique.Numbers(volumes...)...))
}

func (signal *Signal) isPeakScarcity(quoteVol float64, volumes []float64) bool {
	if len(volumes) == 0 {
		return false
	}

	minVolume := float64(
		statistic.NewQuantile(0, stat.LinInterp, nil).Observe(nomagique.Numbers(volumes...)...),
	)

	return quoteVol <= minVolume
}

func medianDepthEvidence(quoteVol, lower, upper float64) float64 {
	if upper <= lower || quoteVol <= lower || quoteVol >= upper {
		return 0
	}

	center := (lower + upper) / 2
	halfBand := (upper - lower) / 2

	if halfBand <= 0 {
		return 0
	}

	distance := math.Abs(quoteVol - center)

	return math.Max(0, 1-distance/halfBand)
}

func (signal *Signal) classify(
	quoteVol, lower, upper float64,
	peakScarcity bool,
	historicallyLiquid bool,
) logic.CategoryType {
	if historicallyLiquid && (peakScarcity || quoteVol <= lower) {
		return logic.CategoryMedianDepth
	}

	if peakScarcity || quoteVol <= lower {
		return logic.CategoryExtremeScarcity
	}

	if quoteVol >= upper {
		return logic.CategoryRobustLiquidity
	}

	return logic.CategoryMedianDepth
}
