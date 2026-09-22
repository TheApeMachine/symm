package store

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/errnie"
)

/* CaptureServer owns one capture session and assigns each ingress frame an identity. */
type CaptureServer struct {
	session  string
	sequence int64
	out      []byte
	record   CaptureRecord
}

/* NewCapture creates an idle capture session; no frame exists until Write. */
func NewCapture() *CaptureServer {
	return &CaptureServer{session: rand.Text()}
}

/* Write encodes ingress metadata and byte-preserving base64 payload for a table row. */
func (server *CaptureServer) Write(ctx context.Context, call Capture_write) error {
	server.out = nil
	payload, err := call.Args().Payload()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "capture: read payload", err))
	}

	if len(payload) == 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "capture: empty frame", nil))
	}

	endpoint, err := call.Args().Endpoint()

	if err != nil || endpoint == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "capture: endpoint is required", err))
	}

	receivedAt, err := call.Args().ReceivedAt()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "capture: read receive time", err))
	}

	if _, err := time.Parse(time.RFC3339Nano, receivedAt); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "capture: receive time must be supplied by ingress", err))
	}

	symbol, err := call.Args().Symbol()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "capture: read symbol", err))
	}

	kind, err := call.Args().Kind()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "capture: read kind", err))
	}

	if server.sequence == math.MaxInt64 {
		return errnie.Error(errnie.Err(errnie.Validation, "capture: session sequence exhausted", nil))
	}

	row := CaptureRecord{
		ID:      fmt.Sprintf("%s:%d", server.session, server.sequence),
		Session: server.session, Sequence: server.sequence,
		ReceivedAt: receivedAt, ReceivedTime: receivedAt, Endpoint: endpoint,
		Symbol: symbol, Kind: kind, Payload: payload,
	}
	encoded, err := json.Marshal(row)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "capture: encode row", err))
	}

	server.sequence++
	server.out = encoded
	server.record = row
	return nil
}

/* Done emits each captured row once. */
func (server *CaptureServer) Done(ctx context.Context, call Capture_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "capture: allocate result", err))
	}

	if err := results.SetOut(server.out); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "capture: set row", err))
	}

	results.SetSequence(server.record.Sequence)
	for _, err := range []error{results.SetPayload(server.record.Payload), results.SetSession(server.record.Session), results.SetEndpoint(server.record.Endpoint)} {
		if err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "capture: frame metadata", err))
		}
	}
	server.record = CaptureRecord{}
	server.out = nil
	return nil
}

/*
	CaptureRecord preserves ingress bytes and the cursor assigned by one capture owner.

ReceivedTime retains nanoseconds independently of Iceberg's timestamp precision.
*/
type CaptureRecord struct {
	ID           string `json:"capture_id"`
	Session      string `json:"capture_session"`
	Sequence     int64  `json:"capture_sequence"`
	ReceivedAt   string `json:"received_at"`
	ReceivedTime string `json:"received_time"`
	Endpoint     string `json:"endpoint"`
	Symbol       string `json:"symbol,omitempty"`
	Kind         string `json:"kind,omitempty"`
	Payload      []byte `json:"payload"`
}
