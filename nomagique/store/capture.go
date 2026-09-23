package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	runtime "github.com/theapemachine/symm/nomagique/runtime"
)

/*
CaptureServer owns one capture session and assigns each ingress frame an identity.
*/
type CaptureServer struct {
	*runtime.System
	session  string
	sequence int64
	rows     []CaptureRecord
}

/*
NewCapture creates an idle capture session; no frame exists until Write.
*/
func NewCapture(ctx context.Context) *CaptureServer {
	return &CaptureServer{
		System:  runtime.NewSystem(ctx, "store.capture"),
		session: rand.Text(),
	}
}

/*
Write admits every frame that arrived, in slot order, as the next rows of the session.
*/
func (server *CaptureServer) Write(ctx context.Context, call Capture_write) error {
	args := call.Args()
	payloads, err := args.Payload()

	if err != nil {
		return server.Error(errnie.Err(
			errnie.Validation, "capture: read payload", err,
		))
	}

	endpoints, err := args.Endpoint()

	if err != nil {
		return server.Error(errnie.Err(
			errnie.Validation, "capture: read endpoint", err,
		))
	}

	times, err := args.ReceivedAt()

	if err != nil {
		return server.Error(errnie.Err(
			errnie.Validation, "capture: read receive time", err,
		))
	}

	symbols, err := args.Symbol()

	if err != nil {
		return server.Error(errnie.Err(
			errnie.Validation, "capture: read symbol", err,
		))
	}

	kinds, err := args.Kind()

	if err != nil {
		return server.Error(errnie.Err(
			errnie.Validation, "capture: read kind", err,
		))
	}

	for slot := range payloads.Len() {
		payload, err := payloads.At(slot)

		if err != nil {
			return server.Error(errnie.Err(
				errnie.Validation, "capture: read payload", err,
			))
		}

		if len(payload) == 0 {
			continue
		}

		row, err := server.admit(
			slot, bytes.Clone(payload), endpoints, times, symbols, kinds,
		)

		if err != nil {
			return err
		}

		server.rows = append(server.rows, row)
	}
	return nil
}

func (server *CaptureServer) admit(
	slot int, payload []byte, endpoints, times, symbols, kinds capnp.TextList,
) (CaptureRecord, error) {
	var metadata [4]string

	for index, list := range []capnp.TextList{endpoints, times, symbols, kinds} {
		value, err := server.slotText(list, slot)

		if err != nil {
			return CaptureRecord{}, server.Error(err)
		}

		metadata[index] = value
	}

	endpoint, receivedAt := metadata[0], metadata[1]

	if endpoint == "" {
		return CaptureRecord{}, server.Error(errnie.Err(
			errnie.Validation, "capture: endpoint is required", nil,
		))
	}

	if _, err := time.Parse(time.RFC3339Nano, receivedAt); err != nil {
		return CaptureRecord{}, server.Error(errnie.Err(
			errnie.Validation,
			"capture: receive time must be supplied by ingress",
			err,
		))
	}

	if server.sequence == math.MaxInt64 {
		return CaptureRecord{}, server.Error(errnie.Err(
			errnie.Validation, "capture: session sequence exhausted", nil,
		))
	}

	row := CaptureRecord{
		ID:           fmt.Sprintf("%s:%d", server.session, server.sequence),
		Session:      server.session,
		Sequence:     server.sequence,
		ReceivedAt:   receivedAt,
		ReceivedTime: receivedAt,
		Endpoint:     endpoint,
		Symbol:       metadata[2],
		Kind:         metadata[3],
		Payload:      payload,
	}

	server.sequence++
	return row, nil
}

/*
slotText reads an optional gathered slot; a slot nothing was wired into is absent.
*/
func (server *CaptureServer) slotText(list capnp.TextList, slot int) (string, error) {
	if slot >= list.Len() {
		return "", nil
	}

	value, err := list.At(slot)

	if err != nil {
		return "", server.Error(errnie.Err(
			errnie.Validation, "capture: frame metadata", err,
		))
	}

	return value, nil
}

/*
Done emits each captured row once, oldest first.
*/
func (server *CaptureServer) Done(ctx context.Context, call Capture_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return server.Error(errnie.Err(
			errnie.Internal, "capture: allocate result", err,
		))
	}

	if len(server.rows) == 0 {
		results.SetIdle()
		return nil
	}

	record := server.rows[0]
	server.rows = server.rows[1:]
	results.SetPending(uint64(len(server.rows)))

	encoded, err := sonic.Marshal(record)

	if err != nil {
		return server.Error(errnie.Err(
			errnie.Internal, "capture: encode row", err,
		))
	}

	results.SetRow()
	row := results.Row()
	row.SetSequence(record.Sequence)

	for _, err := range []error{
		row.SetOut(encoded),
		row.SetPayload(record.Payload),
		row.SetSession(record.Session),
		row.SetEndpoint(record.Endpoint),
	} {
		if err != nil {
			return server.Error(errnie.Err(
				errnie.Internal, "capture: frame metadata", err,
			))
		}
	}

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
