package store

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
)

/* CaptureServer owns one capture session and assigns each ingress frame an identity. */
type CaptureServer struct {
	session  string
	sequence uint64
	out      []byte
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

	row := struct {
		ID         string `json:"capture_id"`
		ReceivedAt string `json:"received_at"`
		Endpoint   string `json:"endpoint"`
		Symbol     string `json:"symbol,omitempty"`
		Kind       string `json:"kind,omitempty"`
		Payload    []byte `json:"payload"`
	}{fmt.Sprintf("%s:%d", server.session, server.sequence), receivedAt, endpoint, symbol, kind, payload}
	encoded, err := json.Marshal(row)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "capture: encode row", err))
	}

	server.sequence++
	server.out = encoded
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

	server.out = nil
	return nil
}
