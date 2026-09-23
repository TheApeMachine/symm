package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	records map[string]CaptureRecord
	ordered []CaptureRecord
	sealed  bool
	cursor  int
}

func NewTape() *TapeServer {
	return &TapeServer{records: make(map[string]CaptureRecord)}
}

/* Write accepts complete archive rows until snapshot exhaustion. */
func (server *TapeServer) Write(ctx context.Context, call Tape_write) error {
	row, err := call.Args().Row()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "tape: read row", err))
	}

	if server.sealed {
		if len(row) != 0 || !call.Args().Exhausted() {
			return errnie.Error(errnie.Err(errnie.Validation, "tape: cannot append to sealed snapshot", nil))
		}
		return nil
	}

	if len(row) != 0 {
		if err := server.append(row); err != nil {
			return err
		}
	}

	if !call.Args().Exhausted() {
		if len(row) == 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "tape: missing archive row", nil))
		}
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

/* Done emits no frames before ordering is complete, then each unique frame once. */
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

	record := server.ordered[server.cursor]
	row, err := json.Marshal(record)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "tape: encode capture row", err))
	}

	results.SetFrame()
	frame := results.Frame()
	frame.SetSequence(record.Sequence)

	for _, err := range []error{frame.SetRow(row), frame.SetPayload(record.Payload), frame.SetSession(record.Session), frame.SetReceivedAt(record.ReceivedTime), frame.SetEndpoint(record.Endpoint)} {
		if err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "tape: set frame", err))
		}
	}

	server.ordered[server.cursor] = CaptureRecord{}
	server.cursor++
	results.SetPending(uint64(len(server.ordered) - server.cursor))
	return nil
}
