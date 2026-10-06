package learning

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Ledger operations. Each arrival on a TemporalLedger is *[3][]float64
{control, features, predictions}; control[0] selects the operation:

	LedgerIssue   {op, step, reference, horizon}  features, predictions per horizon
	LedgerResolve {op, step, reference}
*/
const (
	LedgerIssue = iota
	LedgerResolve
)

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

Each arrival yields *[9]float64 {resolved, total, pending, hasOutcome,
outcomeHorizon, outcomePrediction, outcomeTarget, outcomeError, outcomeStep}.
The outcome is the most recent resolution, retained across arrivals.
*/
type TemporalLedger struct {
	*core.PrimitiveError
	maxHorizon  int
	manifold    core.Primitive
	target      core.Primitive
	features    map[int64][]float64
	predictions map[int64][]float64
	issued      map[int64][3]float64
	references  map[int64]float64
	seq         int64
	oldest      int64
	resolved    int
	total       int
	out         [9]float64
	command     [3][]float64
	control     []float64
	pair        [2]float64
}

/*
NewTemporalLedger constructs a temporal ledger primitive over the manifold it
supervises and the target primitive that maps reference pairs into supervised
targets. The manifold and the target must be supplied: an absent owner is a
shape failure, not a defaulted one.
*/
func NewTemporalLedger(
	maxHorizon int,
	manifold core.Primitive,
	target core.Primitive,
) core.Primitive {
	ledger := &TemporalLedger{
		PrimitiveError: core.NewPrimitiveError(),
		maxHorizon:     maxHorizon,
		manifold:       manifold,
		target:         target,
		features:       make(map[int64][]float64),
		predictions:    make(map[int64][]float64),
		issued:         make(map[int64][3]float64),
		references:     make(map[int64]float64),
		oldest:         1,
		control:        make([]float64, 4),
	}

	if maxHorizon <= 0 {
		ledger.Error(fmt.Errorf("%w: ledger: horizon must be positive", core.ErrDomain))
		return ledger
	}

	if manifold == nil || target == nil {
		ledger.Error(fmt.Errorf(
			"%w: ledger: requires a manifold and a target transform",
			core.ErrShape,
		))
	}

	return ledger
}

func (op *TemporalLedger) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if op.Error() != nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			command := (*[3][]float64)(arriving)
			control := command[0]

			if len(control) < 3 || (control[0] != LedgerIssue && control[0] != LedgerResolve) {
				op.Error(fmt.Errorf(
					"%w: ledger: command must be {issue|resolve, step, reference, ...}",
					core.ErrShape,
				))
				return
			}

			step, reference := control[1], control[2]

			if control[0] == LedgerIssue && reference > 0 && len(command[1]) > 0 {
				op.seq++
				horizon := 1.0

				if len(control) > 3 {
					horizon = control[3]
				}

				horizon = min(max(horizon, 1), float64(op.maxHorizon))
				op.features[op.seq] = append([]float64(nil), command[1]...)
				op.predictions[op.seq] = append([]float64(nil), command[2]...)
				op.issued[op.seq] = [3]float64{reference, horizon, 0}
				op.references[op.seq] = reference

				if op.seq > int64(op.maxHorizon) {
					purgeBelow := op.seq - int64(op.maxHorizon)

					for key := range op.references {
						if key < purgeBelow {
							delete(op.references, key)
						}
					}
				}
			}

			if control[0] == LedgerResolve && reference > 0 && op.seq > 0 {
				refSeq := op.seq + 1
				op.references[refSeq] = reference
				outcomeHorizon := 0

				for key := op.oldest; key <= op.seq; key++ {
					meta, found := op.issued[key]

					if !found {
						continue
					}

					available := min(refSeq-key, int64(op.maxHorizon))
					resolvedHorizon := int(meta[2])

					if available <= int64(resolvedHorizon) {
						break
					}

					for horizon := resolvedHorizon + 1; horizon <= int(available); horizon++ {
						current, found := op.references[key+int64(horizon)]

						if !found {
							break
						}

						op.pair = [2]float64{current, meta[0]}
						target := 0.0

						for out := range op.target.Next(data.NewValue(op.pair)) {
							target = *(*float64)(out)
						}

						if err := op.target.Error(); err != nil {
							op.Error(fmt.Errorf("ledger: resolve failed for horizon %d: %w", horizon, err))
							return
						}

						prediction := 0.0

						if horizon-1 < len(op.predictions[key]) {
							prediction = op.predictions[key][horizon-1]
						}

						op.control = op.control[:4]
						op.control[0] = ManifoldTask
						op.control[1] = float64(horizon)
						op.control[2] = prediction
						op.control[3] = target
						op.command = [3][]float64{op.control, op.features[key], nil}

						for range op.manifold.Next(data.NewValue(op.command)) {
						}

						if err := op.manifold.Error(); err != nil {
							op.Error(fmt.Errorf("ledger: resolve failed for horizon %d: %w", horizon, err))
							return
						}

						meta[2] = float64(horizon)
						op.issued[key] = meta
						op.total++

						if outcomeHorizon == 0 || horizon <= outcomeHorizon {
							outcomeHorizon = horizon
							op.out[3] = 1
							op.out[4] = float64(horizon)
							op.out[5] = prediction
							op.out[6] = target
							op.out[7] = target - prediction
							op.out[8] = step
						}
					}

					if int(meta[2]) >= op.maxHorizon {
						delete(op.issued, key)
						delete(op.features, key)
						delete(op.predictions, key)
						op.resolved++
					}
				}

				for op.oldest <= op.seq {
					if _, found := op.issued[op.oldest]; found {
						break
					}

					op.oldest++
				}
			}

			op.out[0] = float64(op.resolved)
			op.out[1] = float64(op.total)
			op.out[2] = float64(len(op.issued))

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
