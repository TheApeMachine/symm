package learning

import (
	"errors"
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
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
IssueIntent records predictions and feature state for delayed evaluation.
Predictions holds one issued forecast per horizon, indexed by horizon minus
one; the caller retains its own authoritative sequence for the horizon
parameter, which is kept for outcome telemetry.
*/
type IssueIntent struct {
	Step        int64
	Reference   float64
	Features    []float64
	Predictions []float64
	Horizon     int
}

/*
ResolveIntent observes the current reference and supervises every pending
prediction against each horizon whose subsequent reference has arrived.
*/
type ResolveIntent struct {
	Step      int64
	Reference float64
}

/*
LedgerCommand discriminates one ledger operation. Exactly one intent must be
set; anything else is a shape failure.
*/
type LedgerCommand struct {
	Issue   *IssueIntent
	Resolve *ResolveIntent
}

/*
LedgerReading is the ledger's answer: the outcome of one resolution pass and
its retained counts.
*/
type LedgerReading struct {
	Outcome  *ResolutionOutcome
	Resolved int
	Total    int
	Pending  int
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
	err        error
	maxHorizon int
	manifold   core.Primitive
	target     core.Primitive
	pending    map[int64]*PendingReference
	references map[int64]float64
	seq        int64
	oldest     int64
	resolved   int
	total      int
	last       *ResolutionOutcome
	out        LedgerReading
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
	if maxHorizon <= 0 {
		return &TemporalLedger{
			err: fmt.Errorf(
				"%w: ledger: horizon must be positive",
				core.ErrDomain,
			),
		}
	}

	if manifold == nil || target == nil {
		return &TemporalLedger{
			err: fmt.Errorf(
				"%w: ledger: requires a manifold and a target transform",
				core.ErrShape,
			),
		}
	}

	return &TemporalLedger{
		maxHorizon: maxHorizon,
		manifold:   manifold,
		target:     target,
		pending:    make(map[int64]*PendingReference),
		references: make(map[int64]float64),
		oldest:     1,
	}
}

/*
Next receives *LedgerCommand payloads and yields a *LedgerReading for each.
Any invalid intent ends the stream with the error recorded.
*/
func (op *TemporalLedger) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	if op.err != nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			command := (*LedgerCommand)(arriving)

			if (command.Issue == nil) == (command.Resolve == nil) {
				op.Error(fmt.Errorf(
					"%w: ledger: command must set exactly one intent",
					core.ErrShape,
				))
				return
			}

			if command.Issue != nil {
				op.issue(command.Issue)
			}

			if command.Resolve != nil {
				if err := op.resolve(command.Resolve); err != nil {
					op.Error(err)
					return
				}
			}

			op.out = LedgerReading{
				Outcome:  op.last,
				Resolved: op.resolved,
				Total:    op.total,
				Pending:  len(op.pending),
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *TemporalLedger) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
issue records predictions and feature state for delayed evaluation. The ledger
assigns its own strictly increasing sequence so resolution order is
unambiguous.
*/
func (op *TemporalLedger) issue(intent *IssueIntent) {
	if intent.Reference <= 0 || len(intent.Features) == 0 {
		return
	}

	op.seq++

	horizon := intent.Horizon

	if horizon < 1 {
		horizon = 1
	}

	if horizon > op.maxHorizon {
		horizon = op.maxHorizon
	}

	op.pending[op.seq] = &PendingReference{
		Seq:         op.seq,
		Reference:   intent.Reference,
		Features:    append([]float64(nil), intent.Features...),
		Predictions: append([]float64(nil), intent.Predictions...),
		Horizon:     horizon,
	}
	op.references[op.seq] = intent.Reference
	op.prune()
}

/*
resolve observes the current reference and supervises every pending prediction
against each horizon whose subsequent reference has arrived, in issue order.
A row issued at sequence s trains horizon h once the reference at s+h exists,
so one sample per horizon is generated per step regardless of how the external
step numbers jump or repeat. The outcome reports the row's own chosen horizon
once its delayed target arrives.
*/
func (op *TemporalLedger) resolve(intent *ResolveIntent) error {
	if intent.Reference <= 0 || op.seq == 0 || op.maxHorizon < 1 {
		return nil
	}

	refSeq := op.seq + 1
	op.references[refSeq] = intent.Reference

	var outcome *ResolutionOutcome

	for key := op.oldest; key <= op.seq; key++ {
		item, found := op.pending[key]
		if !found {
			continue
		}

		// References are stored for every issued sequence; the row can be
		// supervised up to the horizon whose reference has already arrived.
		available := refSeq - item.Seq

		if available > int64(op.maxHorizon) {
			available = int64(op.maxHorizon)
		}

		if available <= int64(item.Resolved) {
			break
		}

		for horizon := item.Resolved + 1; horizon <= int(available); horizon++ {
			current, found := op.references[item.Seq+int64(horizon)]

			if !found {
				break
			}

			target, err := op.transform(current, item.Reference)

			if err != nil {
				return fmt.Errorf("ledger: resolve failed for horizon %d: %w", horizon, err)
			}

			prediction := 0.0

			if horizon-1 < len(item.Predictions) {
				prediction = item.Predictions[horizon-1]
			}

			if err := op.observeTask(horizon, item.Features, prediction, target); err != nil {
				return fmt.Errorf("ledger: resolve failed for horizon %d: %w", horizon, err)
			}

			item.Resolved = horizon
			op.total++

			if outcome == nil || horizon <= outcome.Horizon {
				outcome = &ResolutionOutcome{
					Horizon:    horizon,
					Prediction: prediction,
					Target:     target,
					Error:      target - prediction,
					Step:       intent.Step,
				}
				op.last = outcome
			}
		}

		if item.Resolved >= op.maxHorizon {
			delete(op.pending, key)
			op.resolved++
		}
	}

	for op.oldest <= op.seq {
		if _, found := op.pending[op.oldest]; found {
			break
		}
		op.oldest++
	}

	return nil
}

/*
transform maps one resolved reference pair into its supervised target through
the configured target primitive.
*/
func (op *TemporalLedger) transform(current, past float64) (float64, error) {
	evaluation := transport.NewEvaluate(op.target)
	var target float64

	for out := range evaluation.Next(transport.NewValues(Observation{
		Current: current,
		Past:    past,
	}).Next(nil)) {
		target = *(*float64)(out)
	}

	if err := evaluation.Error(); err != nil {
		return 0, err
	}

	return target, nil
}

/*
observeTask forwards one supervised sample to the manifold's task head.
*/
func (op *TemporalLedger) observeTask(
	horizon int,
	features []float64,
	prediction float64,
	target float64,
) error {
	evaluation := transport.NewEvaluate(op.manifold)

	for range evaluation.Next(transport.NewValues(ManifoldCommand{
		ObserveTask: &TaskIntent{
			Horizon:    horizon,
			Features:   features,
			Prediction: prediction,
			Target:     target,
		},
	}).Next(nil)) {
	}

	return evaluation.Error()
}

func (op *TemporalLedger) prune() {
	if op.seq <= int64(op.maxHorizon) {
		return
	}

	purgeBelow := op.seq - int64(op.maxHorizon)
	for key := range op.references {
		if key < purgeBelow {
			delete(op.references, key)
		}
	}
}
