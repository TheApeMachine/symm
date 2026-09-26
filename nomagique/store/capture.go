package store

import (
	"bytes"
	"context"
	"fmt"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	runtime "github.com/theapemachine/symm/nomagique/runtime"
)

/*
CaptureServer persists exact raw bytes under their source-owned identities.
*/
type CaptureServer struct {
	*runtime.System
	rows []CaptureRecord
}

/*
NewCapture creates an idle capture session; no frame exists until Write.
*/
func NewCapture(ctx context.Context) *CaptureServer {
	return &CaptureServer{
		System: runtime.NewSystem(ctx, "store.capture"),
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

	provenance, err := args.Provenance()
	if err != nil {
		return server.Error(errnie.Err(errnie.Validation, "capture: provenance", err))
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
			slot, bytes.Clone(payload), provenance, symbols, kinds,
		)

		if err != nil {
			return err
		}

		server.rows = append(server.rows, row)
	}
	return nil
}

func (server *CaptureServer) admit(
	slot int, payload []byte, provenance capnp.DataList, symbols, kinds capnp.TextList,
) (CaptureRecord, error) {
	if slot >= provenance.Len() {
		return CaptureRecord{}, server.Error(errnie.Err(errnie.Validation, "capture: source provenance is required", nil))
	}
	encoded, err := provenance.At(slot)
	if err != nil {
		return CaptureRecord{}, server.Error(err)
	}
	var origin struct {
		Session    string `json:"session"`
		Sequence   *int64 `json:"sequence"`
		Endpoint   string `json:"endpoint"`
		ReceivedAt string `json:"receivedAt"`
	}
	if err := sonic.Unmarshal(encoded, &origin); err != nil {
		return CaptureRecord{}, server.Error(err)
	}
	if origin.Session == "" || origin.Sequence == nil || *origin.Sequence < 0 || origin.Endpoint == "" {
		return CaptureRecord{}, server.Error(errnie.Err(errnie.Validation, "capture: invalid source provenance", nil))
	}
	if _, err := time.Parse(time.RFC3339Nano, origin.ReceivedAt); err != nil {
		return CaptureRecord{}, server.Error(errnie.Err(errnie.Validation, "capture: invalid ingress time", err))
	}
	symbol, err := server.slotText(symbols, slot)
	if err != nil {
		return CaptureRecord{}, err
	}
	kind, err := server.slotText(kinds, slot)
	if err != nil {
		return CaptureRecord{}, err
	}
	return CaptureRecord{
		ID: fmt.Sprintf("%s:%d", origin.Session, *origin.Sequence), Session: origin.Session, Sequence: *origin.Sequence,
		ReceivedAt: origin.ReceivedAt, ReceivedTime: origin.ReceivedAt, Endpoint: origin.Endpoint,
		Symbol: symbol, Kind: kind, Payload: payload,
	}, nil
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
