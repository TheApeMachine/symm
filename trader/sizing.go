package trader

import (
	"math"
	"sync"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/numeric/adaptive"
)

type sourceSlotStats struct {
	wins       adaptive.BernoulliRatio
	payoff     *adaptive.EMA
	calibrator engine.PredictionCalibrator
}

/*
KellySizer adapts slot notional from settled feedback and calibration trust.
*/
type KellySizer struct {
	stateMu  sync.Mutex
	bySource map[string]*sourceSlotStats
	params   engine.CalibrationParams
}

func NewKellySizer(params engine.CalibrationParams) *KellySizer {
	return &KellySizer{
		bySource: make(map[string]*sourceSlotStats),
		params:   params,
	}
}

func (kellySizer *KellySizer) ApplyFeedback(feedback engine.PredictionFeedback) {
	if !engine.ValidPredictionFeedback(feedback) {
		return
	}

	kellySizer.stateMu.Lock()
	defer kellySizer.stateMu.Unlock()

	stats := kellySizer.sourceStats(feedback.Source)
	stats.calibrator.Apply(feedback)
	stats.wins.Observe(feedback.ActualReturn > 0)

	if feedback.PredictedReturn > 0 {
		payoffSample := math.Abs(feedback.ActualReturn / feedback.PredictedReturn)

		if payoffSample > 0 {
			_, _ = stats.payoff.Next(0, payoffSample)
		}
	}
}

func (kellySizer *KellySizer) SlotEUR(
	balance float64,
	source string,
	jointConfidence float64,
	meanError float64,
) float64 {
	if balance <= 0 || jointConfidence <= 0 {
		return 0
	}

	kellySizer.stateMu.Lock()
	stats := kellySizer.sourceStats(source)
	kellySizer.stateMu.Unlock()

	maxFraction := config.System.MaxSlotPct / 100
	fraction := maxFraction

	if stats.wins.Total() >= float64(kellySizer.params.MinCalibrationSamples) {
		winRate := stats.wins.Ratio()
		payoffRatio := stats.payoff.Value()

		if payoffRatio > 0 {
			kelly := (winRate*payoffRatio - (1 - winRate)) / payoffRatio

			if kelly <= 0 {
				return 0
			}

			fraction = kelly * config.System.KellyFraction
		}
	}

	fraction *= jointConfidence * trustScale(meanError) * stats.calibrator.Scale()

	if fraction > maxFraction {
		fraction = maxFraction
	}

	if fraction <= 0 {
		return 0
	}

	slot := balance * fraction

	if slot < config.System.MinCostEUR {
		return 0
	}

	return slot
}

func (kellySizer *KellySizer) sourceStats(source string) *sourceSlotStats {
	stats := kellySizer.bySource[source]

	if stats == nil {
		stats = &sourceSlotStats{
			payoff:     adaptive.NewEMA(0),
			calibrator: engine.NewPredictionCalibrator(kellySizer.params),
		}
		kellySizer.bySource[source] = stats
	}

	return stats
}

func trustScale(meanError float64) float64 {
	if meanError <= 0 {
		return 1
	}

	return 1 / (1 + meanError)
}
