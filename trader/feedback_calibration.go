package trader

import (
	"sync"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/numeric/learned"
)

/*
sourceCalibrator owns one learned.Forecast per signal source plus an
EMA of calibrated confidence per source for the dashboard gauge. The
forecast's Trust() score is the top-down feedback loop the spec
describes: every settled prediction updates the per-source forecast,
the next raw measurement.Confidence from that source is multiplied by
the trust factor before going anywhere (perspective bucket, gauge,
audit), and the system becomes self-tuning at the *signal output*
level without each signal having to grow its own calibrator.

Bootstrapping is handled by engine.TrustCalibratedConfidence —
samples < MinCalibrationSamples pass raw confidence through so
predictions keep flowing while feedback accumulates.

Concurrency: every per-source entry is created at most once via the
mu-guarded sourceStats path; afterward Observe/ApplyFeedback take the
inner *learned.Forecast lock implicitly through Next.
*/
type sourceCalibrator struct {
	mu       sync.RWMutex
	bySource map[string]*sourceForecast
	shock    *regimeShockBreaker
}

type sourceForecast struct {
	forecast *learned.Forecast
}

func newSourceCalibrator(shock *regimeShockBreaker) *sourceCalibrator {
	return &sourceCalibrator{
		bySource: make(map[string]*sourceForecast),
		shock:    shock,
	}
}

func (calibrator *sourceCalibrator) entry(source string) *sourceForecast {
	if source == "" {
		return nil
	}

	calibrator.mu.RLock()
	entry := calibrator.bySource[source]
	calibrator.mu.RUnlock()

	if entry != nil {
		return entry
	}

	calibrator.mu.Lock()
	defer calibrator.mu.Unlock()

	if entry = calibrator.bySource[source]; entry != nil {
		return entry
	}

	entry = &sourceForecast{forecast: learned.NewForecast(0.35)}
	calibrator.bySource[source] = entry

	return entry
}

/*
CalibrateConfidence returns the top-down-adjusted confidence value to
use for one fresh measurement. It does NOT mutate any calibrator state
(that only happens on settled feedback). Returns the raw confidence
when source is unknown so unattributed measurements aren't muted.
*/
func (calibrator *sourceCalibrator) CalibrateConfidence(source string, rawConfidence float64) float64 {
	if calibrator == nil || rawConfidence <= 0 {
		return rawConfidence
	}

	if calibrator.shock != nil && calibrator.shock.Mutes(source) {
		return rawConfidence * calibrator.shock.TrustFloor()
	}

	entry := calibrator.entry(source)

	if entry == nil {
		return rawConfidence
	}

	return engine.TrustCalibratedConfidence(
		rawConfidence,
		entry.forecast.Trust(),
		entry.forecast.Updates(),
		engine.DefaultCalibrationParams().MinCalibrationSamples,
	)
}

/*
ApplyFeedback updates one source's calibrator from a settled
prediction. predicted and actual are the prediction's expected and
realized returns — the same pair already in PredictionFeedback. The
forecast learns the predicted-vs-actual ratio under a weight that
penalises surprises, and Trust() reflects that learned weight.
*/
func (calibrator *sourceCalibrator) ApplyFeedback(source string, predicted, actual float64) {
	if calibrator == nil {
		return
	}

	if calibrator.shock != nil && calibrator.shock.Mutes(source) {
		return
	}

	entry := calibrator.entry(source)

	if entry == nil {
		return
	}

	if _, err := entry.forecast.Next(0, predicted, actual); err != nil {
		return
	}
}

/*
Snapshot returns the (source → {trust, samples, scale}) view of the
calibrator for run_stats emit.
*/
func (calibrator *sourceCalibrator) Snapshot() map[string]map[string]any {
	if calibrator == nil {
		return nil
	}

	calibrator.mu.RLock()
	defer calibrator.mu.RUnlock()

	out := make(map[string]map[string]any, len(calibrator.bySource))

	for source, entry := range calibrator.bySource {
		out[source] = map[string]any{
			"trust":   entry.forecast.Trust(),
			"scale":   entry.forecast.Scale(),
			"samples": entry.forecast.Updates(),
		}
	}

	return out
}
