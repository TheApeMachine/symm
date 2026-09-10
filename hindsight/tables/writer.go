package tables

import (
	"context"
	"sync"

	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

/*
Writer accumulates rows per record family and commits them as Iceberg appends.

Every append is a snapshot plus a metadata write, so rows are buffered rather
than written as they arrive. Callers decide the cadence; this type guarantees
that a family is cleared only after its rows have been appended, that a
payload bound keeps each snapshot inside the catalog's HTTP/object timeout,
and that a family which never reached the catalog is still here afterwards.
*/
type Writer struct {
	catalog *Catalog

	mutex       sync.Mutex
	appendBytes int
	runs        []RunRow
	captures    []CaptureRow
	manifests   []ManifestRow
	witnesses   []WitnessRow
	lifecycle   []LifecycleRow
	decisions   []OutcomeRow
	outcomes    []OutcomeRow
	gaps        []GapRow
}

// NewWriter returns a Writer appending into the given catalog.
func NewWriter(catalog *Catalog) *Writer {
	return &Writer{
		catalog:     catalog,
		appendBytes: viper.GetInt("storage.iceberg.append_bytes"),
	}
}

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
Commit appends every buffered family, one family at a time.

A family is detached only for the append, and only the suffix that has not
been acknowledged is restored. Earlier families that already committed stay
committed. Iceberg retries explicit commit conflicts using the table's
configured budget. A timeout or other unknown outcome on a snapshot that was
already sent is not retried here: repeating it could duplicate that snapshot.
*/
func (w *Writer) Commit(ctx context.Context) error {
	if w.appendBytes < 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[iceberg] append_bytes must be nonnegative",
			nil,
		))
	}

	if err := commitFamily(w, ctx, Runs,
		func() []RunRow { rows := w.runs; w.runs = nil; return rows },
		func(rows []RunRow) { w.runs = append(rows, w.runs...) },
		nil, fillRuns,
	); err != nil {
		return err
	}

	if err := commitFamily(w, ctx, Captures,
		func() []CaptureRow { rows := w.captures; w.captures = nil; return rows },
		func(rows []CaptureRow) { w.captures = append(rows, w.captures...) },
		func(row CaptureRow) int { return len(row.Payload) },
		fillCaptures,
	); err != nil {
		return err
	}

	if err := commitFamily(w, ctx, Manifests,
		func() []ManifestRow { rows := w.manifests; w.manifests = nil; return rows },
		func(rows []ManifestRow) { w.manifests = append(rows, w.manifests...) },
		nil, fillManifests,
	); err != nil {
		return err
	}

	if err := commitFamily(w, ctx, Witnesses,
		func() []WitnessRow { rows := w.witnesses; w.witnesses = nil; return rows },
		func(rows []WitnessRow) { w.witnesses = append(rows, w.witnesses...) },
		func(row WitnessRow) int { return len(row.Payload) },
		fillWitnesses,
	); err != nil {
		return err
	}

	if err := commitFamily(w, ctx, Lifecycle,
		func() []LifecycleRow { rows := w.lifecycle; w.lifecycle = nil; return rows },
		func(rows []LifecycleRow) { w.lifecycle = append(rows, w.lifecycle...) },
		nil, fillLifecycle,
	); err != nil {
		return err
	}

	if err := commitFamily(w, ctx, Decisions,
		func() []OutcomeRow { rows := w.decisions; w.decisions = nil; return rows },
		func(rows []OutcomeRow) { w.decisions = append(rows, w.decisions...) },
		nil, fillOutcomes,
	); err != nil {
		return err
	}

	if err := commitFamily(w, ctx, Outcomes,
		func() []OutcomeRow { rows := w.outcomes; w.outcomes = nil; return rows },
		func(rows []OutcomeRow) { w.outcomes = append(rows, w.outcomes...) },
		nil, fillOutcomes,
	); err != nil {
		return err
	}

	return commitFamily(w, ctx, Gaps,
		func() []GapRow { rows := w.gaps; w.gaps = nil; return rows },
		func(rows []GapRow) { w.gaps = append(rows, w.gaps...) },
		nil, fillGaps,
	)
}

func commitFamily[T any](
	w *Writer,
	ctx context.Context,
	name string,
	take func() []T,
	put func([]T),
	payload func(T) int,
	fill func(*array.RecordBuilder, []T),
) error {
	w.mutex.Lock()
	rows := take()
	w.mutex.Unlock()

	if len(rows) == 0 {
		return nil
	}

	var sizeOf func(int) int

	if payload != nil {
		sizeOf = func(index int) int { return payload(rows[index]) }
	}

	committed, err := w.append(ctx, name, sizeOf, len(rows), func(builder *array.RecordBuilder, start, end int) {
		fill(builder, rows[start:end])
	})

	if committed < len(rows) {
		w.mutex.Lock()
		put(rows[committed:])
		w.mutex.Unlock()
	}

	return err
}

/*
append writes one family's rows as successive Iceberg snapshots, each small
enough for the catalog HTTP call to finish.

The record is built from the loaded table's schema, not from the local schema
literal. A catalog assigns its own field IDs when it creates a table, and the
writer matches columns to the table by ID: building from the literal produces
records whose IDs disagree with the table's, which surfaces as a column being
resolved against the wrong one entirely.
*/
func (w *Writer) append(
	ctx context.Context,
	name string,
	payloadSize func(int) int,
	count int,
	fill func(*array.RecordBuilder, int, int),
) (int, error) {
	committed := 0

	for committed < count {
		end, _, err := span(committed, count, w.appendBytes, payloadSize)

		if err != nil {
			return committed, err
		}

		sent, err := w.appendRange(ctx, name, payloadSize, committed, end, fill)

		if err != nil {
			if sent {
				return end, err
			}

			return committed, err
		}

		committed = end
	}

	return committed, nil
}

func (w *Writer) appendRange(
	ctx context.Context,
	name string,
	payloadSize func(int) int,
	start, end int,
	fill func(*array.RecordBuilder, int, int),
) (bool, error) {
	loaded, err := w.catalog.Load(ctx, name)

	if err != nil {
		return false, err
	}

	count := end - start
	var sizeOf func(int) int

	if payloadSize != nil {
		sizeOf = func(index int) int { return payloadSize(start + index) }
	}

	reader, err := records(loaded.Schema(), count, sizeOf, func(builder *array.RecordBuilder, from, to int) {
		fill(builder, start+from, start+to)
	})

	if err != nil {
		return false, err
	}

	defer reader.Release()

	if _, err := loaded.Append(ctx, reader, nil); err != nil {
		return true, errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to append to "+name,
			err,
		))
	}

	return true, nil
}
