package tables

import (
	"context"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/iceberg-go"
	"github.com/theapemachine/errnie"
)

// Capture reads one exact raw input without loading the rest of its run.
func (catalog *Catalog) Capture(ctx context.Context, run string, sequence int64) (CaptureRow, bool, error) {
	rows, err := catalog.captures(ctx, run, iceberg.EqualTo(iceberg.Reference("sequence"), sequence))
	if err != nil {
		return CaptureRow{}, false, err
	}
	if len(rows) == 0 {
		return CaptureRow{}, false, nil
	}
	return rows[0], true, nil
}

// WitnessesAt selects one capture, optionally restricted to one envelope ordinal.
func (catalog *Catalog) WitnessesAt(ctx context.Context, run, kind string, sequence int64, ordinal *int64) ([]WitnessRow, error) {
	// This Iceberg version cannot bind nested envelope fields in its Arrow
	// residual evaluator. Filter the borrowed batch before copying payloads;
	// the run and artifact-family predicates still prune files in the scan.
	return catalog.witnesses(ctx, run, kind, &sequence, ordinal, nil)
}

// ManifestsAt selects the envelopes produced by one captured input.
func (catalog *Catalog) ManifestsAt(ctx context.Context, run string, sequence int64) ([]ManifestRow, error) {
	return catalog.manifests(ctx, run, &sequence)
}

// WitnessesUnseen filters borrowed Arrow rows before copying large payloads.
// Exact identities, rather than a high-water sequence, preserve late witnesses.
func (catalog *Catalog) WitnessesUnseen(ctx context.Context, run, kind string, seen map[EnvelopeRefRow]bool) ([]WitnessRow, error) {
	return catalog.witnesses(ctx, run, kind, nil, nil, seen)
}

// CaptureReferences batches exact identity/time joins without copying raw payloads.
func (catalog *Catalog) CaptureReferences(ctx context.Context, run string, sequences []int64) (map[int64]CaptureRow, error) {
	rows := make(map[int64]CaptureRow, len(sequences))

	if len(sequences) == 0 {
		return rows, nil
	}
	batches, err := catalog.scan(ctx, Captures,
		[]string{"run", "sequence", "stream", "stream_epoch", "stream_sequence", "received_at"},
		forRun(run), iceberg.IsIn(iceberg.Reference("sequence"), sequences...),
	)

	if err != nil {
		return nil, errnie.Error(err)
	}

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(err)
		}

		for index := range int(batch.NumRows()) {
			row := CaptureRow{Run: str(batch.Column(0), index), Sequence: num(batch.Column(1), index), Stream: str(batch.Column(2), index), StreamEpoch: num(batch.Column(3), index), StreamSequence: num(batch.Column(4), index)}
			micros, defined := when(batch.Column(5), index)

			if !defined {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "rehearsal: captured receive time missing", nil))
			}
			row.ReceivedAt = micros.ToTime(arrow.Microsecond).UTC()
			rows[row.Sequence] = row
		}
	}
	return rows, nil
}
