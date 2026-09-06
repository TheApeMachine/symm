package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
)

/*
RehearsalInputs streams the actual numerical contexts presented to the learner,
once per Grid version. Recorded actions, rewards and wallet state are excluded
by the replay consumer. Producer time controls availability; raw capture identity
is retained as provenance, not substituted for a later processing time.
*/
func (store *SQLite) RehearsalInputs(ctx context.Context, run hindsight.RunID, symbol string, visit func(hindsight.RehearsalInput) error) error {
	rows, err := store.reader.QueryContext(ctx, `SELECT data FROM learning_events INDEXED BY learning_events_symbol
		WHERE run_id=? AND symbol=? AND json_extract(data,'$.kind')='issued'
		AND json_extract(data,'$.mode')='virtual' AND json_extract(data,'$.lane')=0 ORDER BY id`, string(run), symbol)
	if err != nil {
		return err
	}
	defer rows.Close()
	var version uint64
	var previous time.Time
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return err
		}
		var event hindsight.LearningEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return err
		}
		if event.At.Before(previous) {
			return errnie.Err(errnie.Validation, "rehearsal: input producer time moved backwards", nil)
		}
		previous = event.At
		if event.GridVersion == version {
			continue
		}
		version = event.GridVersion
		prefix := 0
		for prefix < len(event.Context) && event.Context[prefix] != 0 {
			prefix++
		}
		if len(event.Quantities) != prefix {
			return errnie.Err(errnie.Validation, "rehearsal: named input quantities are missing", nil)
		}
		input := hindsight.RehearsalInput{Capture: event.Capture, Symbol: event.Symbol, At: event.At,
			GridVersion: event.GridVersion, Context: event.Context[:prefix], Quantities: event.Quantities, Authority: event.Authority}
		if err := visit(input); err != nil {
			return err
		}
	}
	return rows.Err()
}

/*
RehearsalFrames streams raw instrument and Level3 frames in their original
capture order. The bounded run index and read-only pool keep it independent
of the live writer; payloads are released after each callback.
*/
func (store *SQLite) RehearsalFrames(ctx context.Context, run hindsight.RunID, through hindsight.CaptureSequence, visit func(hindsight.RawFrame) error) error {
	rows, err := store.reader.QueryContext(ctx, `SELECT capture_seq, stream, stream_epoch, stream_seq, kind, endpoint, at, data, encoding
		FROM events WHERE run_id=? AND capture_seq<=? AND kind IN ('instrument','level3') ORDER BY capture_seq`, string(run), uint64(through))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		frame := hindsight.RawFrame{Identity: hindsight.CaptureIdentity{Run: run}}
		var encoded []byte
		var encoding, at string
		if err := rows.Scan(&frame.Identity.Sequence, &frame.Identity.Stream, &frame.Identity.StreamEpoch,
			&frame.Identity.StreamSequence, &frame.Kind, &frame.Endpoint, &at, &encoded, &encoding); err != nil {
			return err
		}
		frame.ReceivedAt, err = time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return err
		}
		frame.Payload, err = store.decodePayload(encoded, encoding)
		if err != nil {
			return err
		}
		if err := visit(frame); err != nil {
			return err
		}
	}
	return rows.Err()
}
