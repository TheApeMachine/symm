package learning

import (
	"fmt"
)

/*
PendingReference records features and per-horizon predictions at one issue
sequence to resolve against future reference signals. The sequence is internal
to the ledger: callers hand it delayed observations without having to guarantee
consecutive or unique external step numbers.
*/
type PendingReference struct {
	Seq         int64
	Reference   float64
	Features    []float64
	Predictions []float64
	Horizon     int
	Resolved    int
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

Every issued row is supervised against every horizon whose reference has
arrived: with the references retained per sequence, the row trains horizon h on
the cumulative move from its issue reference to the reference h steps later.
That nested supervision is what makes each task row an honest forecast for its
own horizon rather than a blend of several.
*/
type TemporalLedger struct {
	maxHorizon    int
	transform     TargetTransform
	pending       map[int64]*PendingReference
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
		pending:    make(map[int64]*PendingReference),
		references: make(map[int64]float64),
		oldest:     1,
	}
}

/*
Issue records predictions and feature state for delayed evaluation. Predictions
holds one issued forecast per horizon, indexed by horizon minus one; the caller
retains its own authoritative sequence for the horizon parameter, which is kept
for outcome telemetry. The ledger assigns its own strictly increasing sequence
so resolution order is unambiguous.
*/
func (tl *TemporalLedger) Issue(
	step int64,
	reference float64,
	features []float64,
	predictions []float64,
	horizon int,
) {
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
	predCopy := append([]float64(nil), predictions...)
	tl.pending[tl.seq] = &PendingReference{
		Seq:         tl.seq,
		Reference:   reference,
		Features:    featCopy,
		Predictions: predCopy,
		Horizon:     horizon,
	}
	tl.references[tl.seq] = reference
	tl.prune()
}

/*
Resolve observes the current reference and supervises every pending prediction
against each horizon whose subsequent reference has arrived, in issue order.
A row issued at sequence s trains horizon h once the reference at s+h exists,
so one sample per horizon is generated per step regardless of how the external
step numbers jump or repeat. The outcome reports the row's own chosen horizon
once its delayed target arrives.
*/
func (tl *TemporalLedger) Resolve(
	manifold *ResonanceManifold,
	currentStep int64,
	currentReference float64,
) (*ResolutionOutcome, error) {
	if manifold == nil || !finite(currentReference) || tl.seq == 0 || tl.maxHorizon < 1 {
		return nil, nil
	}

	var outcome *ResolutionOutcome

	for key := tl.oldest; key <= tl.seq; key++ {
		item, found := tl.pending[key]
		if !found {
			continue
		}

		// References are stored for every issued sequence; the row can be
		// supervised up to the horizon whose reference has already arrived.
		available := tl.seq - item.Seq

		if available > int64(tl.maxHorizon) {
			available = int64(tl.maxHorizon)
		}

		if available <= int64(item.Resolved) {
			break
		}

		for horizon := item.Resolved + 1; horizon <= int(available); horizon++ {
			current, found := tl.references[item.Seq+int64(horizon)]

			if !found {
				break
			}

			target, valid := tl.transform(current, item.Reference)

			if !valid {
				continue
			}

			prediction := 0.0

			if horizon-1 < len(item.Predictions) {
				prediction = item.Predictions[horizon-1]
			}

			if err := manifold.ObserveTask(
				horizon,
				item.Features,
				prediction,
				target,
			); err != nil {
				return nil, fmt.Errorf("ledger: resolve failed for horizon %d: %w", horizon, err)
			}

			item.Resolved = horizon

			if horizon == item.Horizon {
				outcome = &ResolutionOutcome{
					Horizon:    item.Horizon,
					Prediction: prediction,
					Target:     target,
					Error:      target - prediction,
					Step:       currentStep,
				}
				tl.last = outcome
			}
		}

		if item.Resolved >= tl.maxHorizon {
			delete(tl.pending, key)
			delete(tl.references, key)
			tl.resolvedCount++
		}
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
