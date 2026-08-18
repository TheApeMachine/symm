package learning

import (
	"fmt"
)

/*
PendingReference records features and a predicted value at a sequence step
to resolve against future reference signals.
*/
type PendingReference struct {
	Step       int64
	Reference  float64
	Features   []float64
	Prediction float64
	Horizon    int
}

/*
ResolutionOutcome reports the result of resolving one delayed horizon.
*/
type ResolutionOutcome struct {
	Horizon    int
	Prediction float64
	Target     float64
	Error      float64
	Step       int64
}

/*
TemporalLedger manages multi-horizon delayed target matching without any domain assumptions.
*/
type TemporalLedger struct {
	maxHorizon     int
	transform      TargetTransform
	pending        map[int64]PendingReference
	references     map[int64]float64
	steps          []int64
	resolvedCount  int
	lastResolution *ResolutionOutcome
}

/*
NewTemporalLedger constructs a temporal ledger with a caller-defined target transform.
*/
func NewTemporalLedger(maxHorizon int, transform TargetTransform) *TemporalLedger {
	if maxHorizon <= 0 {
		maxHorizon = 8
	}
	if transform == nil {
		transform = DirectionalTarget(0)
	}
	return &TemporalLedger{
		maxHorizon: maxHorizon,
		transform:  transform,
		pending:    make(map[int64]PendingReference),
		references: make(map[int64]float64),
		steps:      make([]int64, 0, 128),
	}
}

/*
Issue records a prediction and feature state for delayed evaluation.
*/
func (tl *TemporalLedger) Issue(step int64, reference float64, features []float64, prediction float64, horizon int) {
	if step <= 0 || !finite(reference) || len(features) == 0 {
		return
	}

	featCopy := append([]float64(nil), features...)
	tl.pending[step] = PendingReference{
		Step:       step,
		Reference:  reference,
		Features:   featCopy,
		Prediction: prediction,
		Horizon:    horizon,
	}
	tl.references[step] = reference
	tl.steps = append(tl.steps, step)
	tl.prune()
}

/*
Resolve matches previous steps against the current reference value using the configured TargetTransform.
*/
func (tl *TemporalLedger) Resolve(manifold *ResonanceManifold, currentStep int64, currentReference float64) (*ResolutionOutcome, error) {
	if manifold == nil || currentStep <= 0 || !finite(currentReference) {
		return nil, nil
	}

	var latest *ResolutionOutcome

	for horizon := 1; horizon <= tl.maxHorizon; horizon++ {
		targetStep := currentStep - int64(horizon)
		item, found := tl.pending[targetStep]
		if !found {
			continue
		}

		target, valid := tl.transform(currentReference, item.Reference)
		if !valid {
			continue
		}

		// Update the RLS readout head with the generated target
		err := manifold.ObserveTask(
			item.Features,
			[]float64{item.Prediction},
			[]float64{target},
		)
		if err != nil {
			return nil, fmt.Errorf("ledger: resolve failed for horizon %d: %w", horizon, err)
		}

		res := &ResolutionOutcome{
			Horizon:    horizon,
			Prediction: item.Prediction,
			Target:     target,
			Error:      target - item.Prediction,
			Step:       currentStep,
		}
		latest = res
		tl.lastResolution = res
		tl.resolvedCount++

		delete(tl.pending, targetStep)
	}

	return latest, nil
}

func (tl *TemporalLedger) prune() {
	if len(tl.steps) > 256 {
		cutoff := tl.steps[len(tl.steps)-128]
		filtered := tl.steps[:0]
		for _, s := range tl.steps {
			if s >= cutoff {
				filtered = append(filtered, s)
			} else {
				delete(tl.references, s)
				delete(tl.pending, s)
			}
		}
		tl.steps = filtered
	}
}

func (tl *TemporalLedger) ResolvedCount() int {
	return tl.resolvedCount
}

func (tl *TemporalLedger) LastResolution() *ResolutionOutcome {
	return tl.lastResolution
}
