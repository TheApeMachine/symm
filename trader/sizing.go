package trader

import (
	"math"
	"strings"
	"sync"
	"time"

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
	// meanHorizon is the running-mean settled runway (seconds) across all
	// feedback. It is the data-derived reference for the time-preference term in
	// SlotEUR -- no fixed "expected hold" constant.
	meanHorizon *adaptive.EMA
	params      engine.CalibrationParams
}

func NewKellySizer(params engine.CalibrationParams) *KellySizer {
	return &KellySizer{
		bySeries:    make(map[sourceSlotKey]*sourceSlotStats),
		meanHorizon: adaptive.NewEMA(0),
		params:      params,
	}
}

// roundTripFeeReturn is the fee-only round-trip cost as a return fraction. A
// "win" must clear this, not merely zero, or Kelly learns the base rate of
// up-ticks instead of the signal's edge.
func roundTripFeeReturn() float64 {
	feePct := config.System.TakerFeePct * 2

	if config.System.UseMakerEntries {
		feePct = config.System.MakerFeePct + config.System.TakerFeePct
	}

	return feePct / 100
}

func (kellySizer *KellySizer) ApplyFeedback(feedback engine.PredictionFeedback) {
	if !engine.ValidPredictionFeedback(feedback) {
		return
	}

	kellySizer.stateMu.Lock()
	defer kellySizer.stateMu.Unlock()

	stats := kellySizer.sourceStats(feedback.Source, feedback.Regime)
	stats.calibrator.Apply(feedback)
	stats.wins.Observe(feedback.ActualReturn > roundTripFeeReturn())

	// Track how long settled predictions actually run so SlotEUR has a
	// data-derived reference horizon for its time-preference term.
	if feedback.Runway > 0 {
		_, _ = kellySizer.meanHorizon.Next(0, feedback.Runway.Seconds())
	}

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
	horizon time.Duration,
) float64 {
	if balance <= 0 || jointConfidence <= 0 {
		return 0
	}

	kellySizer.stateMu.Lock()
	stats := kellySizer.sourceStats(source, regime)
	meanHorizon := kellySizer.meanHorizon.Value()
	kellySizer.stateMu.Unlock()

	maxFraction := config.System.MaxSlotPct / 100

	// Beta(1,1)-shrunk win rate: (hits+1)/(trials+2). With no settlements this
	// is 0.5 -- the no-information prior, which yields a non-positive Kelly and
	// therefore zero size -- and it converges to the empirical rate as feedback
	// accumulates. This replaces the old hard MinCalibrationSamples cliff: thin
	// evidence shrinks the bet toward zero continuously instead of switching it
	// off at an arbitrary count.
	winRate := (stats.wins.Hits() + 1) / (stats.wins.Total() + 2)

	// Payoff is neutral (1:1) until the EMA has learned a ratio, so a fresh slot
	// sizes on its (shrunk) win rate rather than being blocked outright.
	payoffRatio := stats.payoff.Value()

	if payoffRatio <= 0 {
		payoffRatio = 1
	}

	kelly := (winRate*payoffRatio - (1 - winRate)) / payoffRatio

	if kelly <= 0 {
		return 0
	}

	fraction := kelly * config.System.KellyFraction
	fraction *= jointConfidence * trustScale(meanError) * stats.calibrator.ScaleFor(regime)

	// Time preference -- the "minimize time" half of the objective. For equal
	// edge, commit more capital to faster-recycling trades and less to slow
	// ones: capital freed sooner compounds sooner. The reference is the
	// running-mean settled horizon (data-derived, not a constant), so a hold at
	// the average horizon is unscaled, shorter holds are up-weighted, longer
	// holds down-weighted.
	if horizon > 0 && meanHorizon > 0 {
		fraction *= meanHorizon / horizon.Seconds()
	}

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
