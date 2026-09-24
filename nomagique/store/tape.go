package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/theapemachine/errnie"
)

/*
	TapeServer owns an offline snapshot's capture identity and replay order.

Independent sessions are contiguous and lexically ordered only for reproducibility.
Consumers must key state by session; no ordering across capture owners is implied.
*/
type TapeServer struct {
	records  map[string]CaptureRecord
	ordered  []CaptureRecord
	sealed   bool
	cursor   int
	envelope string
}

func NewTape() *TapeServer {
	return &TapeServer{records: make(map[string]CaptureRecord)}
}

/* Write accepts complete archive rows until snapshot exhaustion. */
func (server *TapeServer) Write(ctx context.Context, call Tape_write) error {
	// A snapshot batch arrives whole and is read once, inside this process; the
	// traversal limit that guards against hostile remote messages would only
	// cap how large a batch may be.
	call.Args().Message().ResetReadLimit(math.MaxUint64)
	rows, err := call.Args().Rows()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "tape: read rows", err))
	}

	envelope, err := call.Args().Envelope()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "tape: read envelope", err))
	}

	if envelope == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "tape: envelope names no field for the frame", nil))
	}

	server.envelope = envelope

	if server.sealed {
		if rows.Len() != 0 || !call.Args().Exhausted() {
			return errnie.Error(errnie.Err(errnie.Validation, "tape: cannot append to sealed snapshot", nil))
		}
		return nil
	}

	for index := range rows.Len() {
		row, err := rows.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "tape: read row", err))
		}

		if len(row) == 0 {
			continue
		}

		if err := server.append(row); err != nil {
			return err
		}
	}

	if !call.Args().Exhausted() {
		return nil
	}

	for _, record := range server.records {
		server.ordered = append(server.ordered, record)
	}
	sort.Slice(server.ordered, func(left, right int) bool {
		if server.ordered[left].Session != server.ordered[right].Session {
			return server.ordered[left].Session < server.ordered[right].Session
		}
		return server.ordered[left].Sequence < server.ordered[right].Sequence
	})
	server.records = nil
	server.sealed = true
	return nil
}

/* append validates explicit cursors and rejects conflicting duplicate records. */
func (server *TapeServer) append(row []byte) error {
	var fields map[string]json.RawMessage

	if err := json.Unmarshal(row, &fields); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "tape: decode archive row", err))
	}

	for _, field := range []string{"capture_id", "capture_session", "capture_sequence", "received_time", "endpoint", "payload"} {
		if len(fields[field]) == 0 || bytes.Equal(fields[field], []byte("null")) {
			return errnie.Error(errnie.Err(errnie.Validation, "tape: missing "+field, nil))
		}
	}

	var record CaptureRecord

	if err := json.Unmarshal(row, &record); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "tape: decode capture", err))
	}

	if record.Session == "" || record.Sequence < 0 || record.Endpoint == "" || len(record.Payload) == 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "tape: invalid capture identity or payload", nil))
	}

	if record.ID != fmt.Sprintf("%s:%d", record.Session, record.Sequence) {
		return errnie.Error(errnie.Err(errnie.Validation, "tape: capture ID disagrees with explicit cursor", nil))
	}

	if _, err := time.Parse(time.RFC3339Nano, record.ReceivedTime); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "tape: invalid receive time", err))
	}

	existing, found := server.records[record.ID]

	if found {
		if existing.ReceivedTime != record.ReceivedTime || existing.Endpoint != record.Endpoint || existing.Symbol != record.Symbol || existing.Kind != record.Kind || !bytes.Equal(existing.Payload, record.Payload) {
			return errnie.Error(errnie.Err(errnie.Validation, "tape: conflicting duplicate "+record.ID, nil))
		}
		return nil
	}

	server.records[record.ID] = record
	return nil
}

/*
Done emits no frames before ordering is complete, then one captured second of
one session per evaluation, each unique frame once.
*/
func (server *TapeServer) Done(ctx context.Context, call Tape_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "tape: allocate result", err))
	}

	if !server.sealed {
		results.SetIdle()
		return nil
	}

	if server.cursor == len(server.ordered) {
		results.SetExhausted()
		results.SetFinished(true)
		return nil
	}

	first := server.ordered[server.cursor]
	second, err := server.second(first)

	if err != nil {
		return err
	}

	// Sockets are read side by side, so a frame from a lagging one can carry an
	// earlier receive time than the frame sequenced before it. The run ends at
	// the first frame received once the capture clock has ticked past the
	// second the run began in.
	tick := second.Add(time.Second)
	end := server.cursor + 1

	for end < len(server.ordered) && server.ordered[end].Session == first.Session {
		next, err := server.second(server.ordered[end])

		if err != nil {
			return err
		}

		if !next.Before(tick) {
			break
		}

		end++
	}

	chunk := server.ordered[server.cursor:end]
	results.SetFrames()
	frames := results.Frames()

	if err := frames.SetSession(first.Session); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "tape: set session", err))
	}

	if err := server.fill(frames, chunk); err != nil {
		return err
	}

	for index := range chunk {
		chunk[index] = CaptureRecord{}
	}

	server.cursor = end
	results.SetPending(uint64(len(server.ordered) - server.cursor))
	return nil
}

/*
second is the capture-clock second a frame was received in.
*/
func (server *TapeServer) second(record CaptureRecord) (time.Time, error) {
	received, err := time.Parse(time.RFC3339Nano, record.ReceivedTime)

	if err != nil {
		return time.Time{}, errnie.Error(errnie.Err(errnie.Validation, "tape: invalid receive time", err))
	}

	return received.Truncate(time.Second), nil
}

/*
fill writes a run of frames into the evaluation's lists, slot by slot.
*/
func (server *TapeServer) fill(frames TapeResult_frames, chunk []CaptureRecord) error {
	count := int32(len(chunk))
	payloads, err := frames.NewPayload(count)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "tape: allocate payloads", err))
	}

	sequences, err := frames.NewSequence(count)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "tape: allocate sequences", err))
	}

	received, err := frames.NewReceivedAt(count)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "tape: allocate receive times", err))
	}

	endpoints, err := frames.NewEndpoint(count)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "tape: allocate endpoints", err))
	}

	documents, err := frames.NewDocuments(count)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "tape: allocate documents", err))
	}

	for slot, record := range chunk {
		document, err := json.Marshal(map[string]any{
			"capture": map[string]string{
				"session":    record.Session,
				"endpoint":   record.Endpoint,
				"receivedAt": record.ReceivedTime,
			},
			"cursor":        map[string]int64{"sequence": record.Sequence},
			server.envelope: json.RawMessage(record.Payload),
		})

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "tape: frame "+record.ID+" is not a document", err))
		}

		sequences.Set(slot, record.Sequence)

		for _, err := range []error{
			payloads.Set(slot, record.Payload),
			received.Set(slot, record.ReceivedTime),
			endpoints.Set(slot, record.Endpoint),
			documents.Set(slot, document),
		} {
			if err != nil {
				return errnie.Error(errnie.Err(errnie.Internal, "tape: set frame", err))
			}
		}
	}

	return nil
}
