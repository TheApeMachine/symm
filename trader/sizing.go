package trader

import (
	"math"
	"strings"
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

type sourceSlotKey struct {
	source string
	regime string
}

/*
KellySizer adapts slot notional from settled feedback and calibration trust.
*/
type KellySizer struct {
	stateMu  sync.Mutex
	bySeries map[sourceSlotKey]*sourceSlotStats
	params   engine.CalibrationParams
}

func NewKellySizer(params engine.CalibrationParams) *KellySizer {
	return &KellySizer{
		bySeries: make(map[sourceSlotKey]*sourceSlotStats),
		params:   params,
	}
}

func (kellySizer *KellySizer) ApplyFeedback(feedback engine.PredictionFeedback) {
	if !engine.ValidPredictionFeedback(feedback) {
		return
	}

	kellySizer.stateMu.Lock()
	defer kellySizer.stateMu.Unlock()

	stats := kellySizer.sourceStats(feedback.Source, feedback.Regime)
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
	regime string,
	jointConfidence float64,
	meanError float64,
) float64 {
	if balance <= 0 || jointConfidence <= 0 {
		return 0
	}

	kellySizer.stateMu.Lock()
	stats := kellySizer.sourceStats(source, regime)
	kellySizer.stateMu.Unlock()

	maxFraction := config.System.MaxSlotPct / 100

	// No cold-start trading: predictions are always recorded (spec step 4),
	// feedback flows even without entries (spec step 6), so we wait until this
	// (source, regime) slot has actually seen its settlements before risking
	// capital on it. The calibrator learns from non-traded predictions.
	if stats.wins.Total() < float64(kellySizer.params.MinCalibrationSamples) {
		return 0
	}

	winRate := stats.wins.Ratio()
	payoffRatio := stats.payoff.Value()

	if payoffRatio <= 0 {
		return 0
	}

	kelly := (winRate*payoffRatio - (1 - winRate)) / payoffRatio

	if kelly <= 0 {
		return 0
	}

	fraction := kelly * config.System.KellyFraction
	fraction *= jointConfidence * trustScale(meanError) * stats.calibrator.ScaleFor(regime)

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

func (kellySizer *KellySizer) sourceStats(source, regime string) *sourceSlotStats {
	key := sourceSlotKey{
		source: strings.TrimSpace(source),
		regime: engine.CalibrationRegime(regime),
	}
	stats := kellySizer.bySeries[key]

	if stats == nil {
		stats = &sourceSlotStats{
			payoff:     adaptive.NewEMA(0),
			calibrator: engine.NewPredictionCalibrator(kellySizer.params),
		}
		kellySizer.bySeries[key] = stats
	}

	return stats
}

func trustScale(meanError float64) float64 {
	if meanError <= 0 {
		return 1
	}

	return 1 / (1 + meanError)
}

/*
SlotDistribution returns one entry per (source, regime) the sizer has
seen, holding the live win rate, payoff ratio, calibrator scale, and
sample count. This is what emitRunStats publishes alongside the global
counters so a post-run analysis can answer "which source-regime
combinations are actually generating sized slots and which are stuck
under MinCalibrationSamples".
*/
func (kellySizer *KellySizer) SlotDistribution() []map[string]any {
	if kellySizer == nil {
		return nil
	}

	kellySizer.stateMu.Lock()
	defer kellySizer.stateMu.Unlock()

	rows := make([]map[string]any, 0, len(kellySizer.bySeries))

	for key, slotStats := range kellySizer.bySeries {
		rows = append(rows, map[string]any{
			"source":           key.source,
			"regime":           key.regime,
			"win_rate":         slotStats.wins.Ratio(),
			"sample_count":     slotStats.wins.Total(),
			"payoff_ratio":     slotStats.payoff.Value(),
			"calibrator_scale": slotStats.calibrator.ScaleFor(key.regime),
		})
	}

	return rows
}
