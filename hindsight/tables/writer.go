package tables

import (
	"context"
	"sync"

	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/iceberg-go"
	"github.com/theapemachine/errnie"
)

/*
Writer accumulates rows per record family and commits them as Iceberg appends.

Every append is a snapshot plus a metadata write, so rows are buffered rather
than written as they arrive: one commit covers a whole batch across every
family it touched. Callers decide the cadence; this type only guarantees that
what it holds is written exactly once and in arrival order within a family.
*/
type Writer struct {
	catalog *Catalog

	mutex     sync.Mutex
	runs      []RunRow
	captures  []CaptureRow
	manifests []ManifestRow
	witnesses []WitnessRow
	lifecycle []LifecycleRow
	decisions []OutcomeRow
	outcomes  []OutcomeRow
	gaps      []GapRow
}

// NewWriter returns a Writer appending into the given catalog.
func NewWriter(catalog *Catalog) *Writer { return &Writer{catalog: catalog} }

// AddRun buffers one process capture session.
func (w *Writer) AddRun(row RunRow) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.runs = append(w.runs, row)
}

// AddCapture buffers one raw external input.
func (w *Writer) AddCapture(row CaptureRow) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.captures = append(w.captures, row)
}

// AddManifest buffers one envelope manifest.
func (w *Writer) AddManifest(row ManifestRow) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.manifests = append(w.manifests, row)
}

// AddWitness buffers one artifact witness.
func (w *Writer) AddWitness(row WitnessRow) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.witnesses = append(w.witnesses, row)
}

// AddLifecycle buffers one position or order transition.
func (w *Writer) AddLifecycle(row LifecycleRow) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.lifecycle = append(w.lifecycle, row)
}

// AddDecision buffers one decision as the agent made it, before grading.
func (w *Writer) AddDecision(row OutcomeRow) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.decisions = append(w.decisions, row)
}

// AddOutcome buffers one graded decision.
func (w *Writer) AddOutcome(row OutcomeRow) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.outcomes = append(w.outcomes, row)
}

// AddGap buffers one capture-integrity gap.
func (w *Writer) AddGap(row GapRow) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.gaps = append(w.gaps, row)
}

// Pending reports how many rows are buffered across every family, so a caller
// can drive commit cadence on volume rather than only on time.
func (w *Writer) Pending() int {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	return len(w.runs) + len(w.captures) + len(w.manifests) +
		len(w.witnesses) + len(w.lifecycle) + len(w.decisions) + len(w.outcomes) + len(w.gaps)
}

/*
Commit appends every buffered family and clears what it wrote.

The buffers are detached under the lock and written outside it, so producers
are never blocked on S3. A family that fails leaves the remaining families
unattempted and its own rows dropped: Iceberg commits are atomic per table, so
a failure means that table saw nothing, and retrying from here would risk
double-writing families that already succeeded.
*/
func (w *Writer) Commit(ctx context.Context) error {
	w.mutex.Lock()
	runs, captures, manifests := w.runs, w.captures, w.manifests
	witnesses, lifecycle, outcomes := w.witnesses, w.lifecycle, w.outcomes
	decisions, gaps := w.decisions, w.gaps
	w.runs, w.captures, w.manifests = nil, nil, nil
	w.witnesses, w.lifecycle, w.outcomes = nil, nil, nil
	w.decisions, w.gaps = nil, nil
	w.mutex.Unlock()

	if len(runs) > 0 {
		if err := w.append(ctx, Runs, RunsSchema(), len(runs), func(b *array.RecordBuilder) {
			fillRuns(b, runs)
		}); err != nil {
			return err
		}
	}

	if len(captures) > 0 {
		if err := w.append(ctx, Captures, CapturesSchema(), len(captures), func(b *array.RecordBuilder) {
			fillCaptures(b, captures)
		}); err != nil {
			return err
		}
	}

	if len(manifests) > 0 {
		if err := w.append(ctx, Manifests, ManifestsSchema(), len(manifests), func(b *array.RecordBuilder) {
			fillManifests(b, manifests)
		}); err != nil {
			return err
		}
	}

	if len(witnesses) > 0 {
		if err := w.append(ctx, Witnesses, WitnessesSchema(), len(witnesses), func(b *array.RecordBuilder) {
			fillWitnesses(b, witnesses)
		}); err != nil {
			return err
		}
	}

	if len(lifecycle) > 0 {
		if err := w.append(ctx, Lifecycle, LifecycleSchema(), len(lifecycle), func(b *array.RecordBuilder) {
			fillLifecycle(b, lifecycle)
		}); err != nil {
			return err
		}
	}

	if len(decisions) > 0 {
		if err := w.append(ctx, Decisions, DecisionsSchema(), len(decisions), func(b *array.RecordBuilder) {
			fillOutcomes(b, decisions)
		}); err != nil {
			return err
		}
	}

	if len(outcomes) > 0 {
		if err := w.append(ctx, Outcomes, OutcomesSchema(), len(outcomes), func(b *array.RecordBuilder) {
			fillOutcomes(b, outcomes)
		}); err != nil {
			return err
		}
	}

	if len(gaps) > 0 {
		if err := w.append(ctx, Gaps, GapsSchema(), len(gaps), func(b *array.RecordBuilder) {
			fillGaps(b, gaps)
		}); err != nil {
			return err
		}
	}

	return nil
}

/*
append writes one family's rows as a single Iceberg snapshot.

The record is built from the loaded table's schema, not from the local schema
literal. A catalog assigns its own field IDs when it creates a table, and the
writer matches columns to the table by ID: building from the literal produces
records whose IDs disagree with the table's, which surfaces as a column being
resolved against the wrong one entirely.
*/
func (w *Writer) append(
	ctx context.Context,
	name string,
	_ *iceberg.Schema,
	count int,
	fill func(*array.RecordBuilder),
) error {
	loaded, err := w.catalog.Load(ctx, name)

	if err != nil {
		return err
	}

	reader, err := records(loaded.Schema(), count, fill)

	if err != nil {
		return err
	}

	defer reader.Release()

	if _, err := loaded.Append(ctx, reader, nil); err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to append to "+name,
			err,
		))
	}

	return nil
}
