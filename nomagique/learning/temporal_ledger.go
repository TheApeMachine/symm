package learning

import (
	"fmt"
)

/*
PendingReference records features and a predicted value at one issue sequence
to resolve against future reference signals. The sequence is internal to the
ledger: callers hand it delayed observations without having to guarantee
consecutive or unique external step numbers.
*/
type PendingReference struct {
	Seq        int64
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
TemporalLedger manages delayed target matching without any domain assumptions.
Issue and Resolve walk an internal monotonic sequence rather than the caller
supplied step, so a burst of observations sharing one external step number can
no longer overwrite an unresolved prediction before its reference arrives.
*/
type TemporalLedger struct {
	maxHorizon    int
	transform     TargetTransform
	pending       map[int64]PendingReference
	references    map[int64]float64
	seq           int64
	oldest        int64
	resolvedCount int
	last          *ResolutionOutcome
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
		oldest:     1,
	}
}

/*
Issue records a prediction and feature state for delayed evaluation. The
caller's step is retained only for outcome telemetry; the ledger assigns its
own strictly increasing sequence so resolution order is unambiguous.
*/
func (tl *TemporalLedger) Issue(step int64, reference float64, features []float64, prediction float64, horizon int) {
	if !finite(reference) || len(features) == 0 {
		return
	}

	tl.seq++
	if horizon < 1 {
		horizon = 1
	}
	if horizon > tl.maxHorizon {
		horizon = tl.maxHorizon
	}

	featCopy := append([]float64(nil), features...)
	tl.pending[tl.seq] = PendingReference{
		Seq:        tl.seq,
		Reference:  reference,
		Features:   featCopy,
		Prediction: prediction,
		Horizon:    horizon,
	}
	tl.references[tl.seq] = reference
	tl.prune()
}

/*
Resolve observes the current reference and settles every pending prediction
whose issued horizon has elapsed, in issue order. A prediction is held until
its horizon of subsequent references has arrived, so a coder that steps once
per reference resolves one sample per step regardless of how the external
step numbers jump or repeat.
*/
func (tl *TemporalLedger) Resolve(manifold *ResonanceManifold, currentStep int64, currentReference float64) (*ResolutionOutcome, error) {
	if manifold == nil || !finite(currentReference) || tl.seq == 0 || tl.maxHorizon < 1 {
		return nil, nil
	}

	var outcome *ResolutionOutcome

	for key := tl.oldest; key <= tl.seq; key++ {
		item, found := tl.pending[key]
		if !found {
			continue
		}

		// The current reference is the first one observed after an issue at
		// seq=item.Seq counts as age 1; a prediction with horizon h is due
		// once h subsequent references have arrived.
		age := tl.seq - item.Seq + 1

		if age < int64(item.Horizon) {
			break
		}

		target, valid := tl.transform(currentReference, item.Reference)

		if !valid {
			continue
		}

		// Update the RLS readout head with the generated target
		if err := manifold.ObserveTask(
			item.Features,
			[]float64{item.Prediction},
			[]float64{target},
		); err != nil {
			return nil, fmt.Errorf("ledger: resolve failed for horizon %d: %w", item.Horizon, err)
		}

		outcome = &ResolutionOutcome{
			Horizon:    item.Horizon,
			Prediction: item.Prediction,
			Target:     target,
			Error:      target - item.Prediction,
			Step:       currentStep,
		}
		tl.last = outcome
		tl.resolvedCount++

		delete(tl.pending, key)
		delete(tl.references, key)
	}

	tl.oldest = tl.seq - int64(tl.maxHorizon) + 1
	if tl.oldest < 1 {
		tl.oldest = 1
	}

	return outcome, nil
}

func (tl *TemporalLedger) prune() {
	if tl.seq-tl.oldest <= int64(tl.maxHorizon) {
		return
	}

	purgeBelow := tl.seq - int64(tl.maxHorizon)

	for key := tl.oldest; key < purgeBelow; key++ {
		delete(tl.pending, key)
		delete(tl.references, key)
	}

	tl.oldest = purgeBelow
}

func (tl *TemporalLedger) ResolvedCount() int {
	return tl.resolvedCount
}

func (tl *TemporalLedger) LastResolution() *ResolutionOutcome {
	return tl.last
}
