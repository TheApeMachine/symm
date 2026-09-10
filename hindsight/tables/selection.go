package tables

import (
	"bytes"
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

/*
ReadStatePayload supplies the persisted EnvelopeState for one exact envelope
identity, which is the state the running binary actually held there. It is the
whole of hindsight.StateReader, so an as-of view resolves through the catalog
itself rather than through whichever surface happens to be asking.
*/
func (catalog *Catalog) ReadStatePayload(run string, sequence, ordinal uint64) ([]byte, bool, error) {
	row, found, err := catalog.StateAt(context.Background(), run, sequence, ordinal)

	return row.Payload, found, err
}

/*
StateAt selects the persisted EnvelopeState witness for one exact envelope
identity. An identity the run carries no state for is absent, not an error.
*/
func (catalog *Catalog) StateAt(
	ctx context.Context, run string, sequence, ordinal uint64,
) (WitnessRow, bool, error) {
	requested := int64(ordinal)
	rows, err := catalog.WitnessesAt(ctx, run, "state", int64(sequence), &requested)

	if err != nil {
		return WitnessRow{}, false, errnie.Error(err)
	}

	for _, row := range rows {
		if uint64(row.Envelope.Sequence) == sequence && uint64(row.Envelope.Ordinal) == ordinal {
			return row, true, nil
		}
	}

	return WitnessRow{}, false, nil
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

/*
WitnessPayloads copies the payloads of one artifact family at the requested
envelope identities, in a single scan of the run.

Identities the run does not carry are absent from the result, not an error.
Payloads of identities that were not requested are never copied, so a
rehearsal walk does not resident the rest of the run. One scan replaces the
per-identity WitnessesAt round trip, which re-planned the same files once per
capture.
*/
func (catalog *Catalog) WitnessPayloads(
	ctx context.Context, run, kind string, wanted []EnvelopeRefRow,
) (map[EnvelopeRefRow][]byte, error) {
	payloads := make(map[EnvelopeRefRow][]byte, len(wanted))

	if err := catalog.EachWitnessPayload(ctx, run, kind, wanted, func(identity EnvelopeRefRow, payload []byte) error {
		payloads[identity] = bytes.Clone(payload)

		return nil
	}); err != nil {
		return nil, err
	}

	return payloads, nil
}

/*
EachWitnessPayload visits matching witness payloads without retaining them.

The payload slice is borrowed from the Arrow batch and is invalid after visit
returns. Callers that need the bytes must copy them inside visit.
*/
func (catalog *Catalog) EachWitnessPayload(
	ctx context.Context, run, kind string, wanted []EnvelopeRefRow,
	visit func(EnvelopeRefRow, []byte) error,
) error {
	if len(wanted) == 0 {
		return nil
	}
	requested := make(map[EnvelopeRefRow]struct{}, len(wanted))

	for _, identity := range wanted {
		requested[identity] = struct{}{}
	}
	filters := []iceberg.BooleanExpression{forRun(run)}

	if kind != "" {
		filters = append(filters, iceberg.EqualTo(iceberg.Reference("artifact_kind"), kind))
	}
	batches, err := catalog.scan(ctx, Witnesses, []string{"envelope", "payload"}, filters...)

	if err != nil {
		return err
	}

	for batch, err := range batches {
		if err != nil {
			return errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] witnesses batch", err))
		}
		envelope, err := named(batch, "envelope")

		if err != nil {
			return err
		}
		payload, err := named(batch, "payload")

		if err != nil {
			return err
		}

		for index := range int(batch.NumRows()) {
			identity := ref(envelope, index)

			if _, known := requested[identity]; !known {
				continue
			}
			held := rawBin(payload, index)

			if len(held) == 0 {
				continue
			}

			if err := visit(identity, held); err != nil {
				return err
			}
		}
	}

	return nil
}

func named(batch arrow.RecordBatch, name string) (arrow.Array, error) {
	for index, field := range batch.Schema().Fields() {
		if field.Name == name {
			return batch.Column(index), nil
		}
	}

	return nil, errnie.Error(errnie.Err(
		errnie.Validation,
		"[iceberg] witnesses column "+name+" missing from projection",
		nil,
	))
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
