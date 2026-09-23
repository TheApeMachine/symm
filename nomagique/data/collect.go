package data

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/theapemachine/errnie"
)

/* CollectServer retains ordered JSON values until an explicit graph boundary. */
type CollectServer struct {
	values []json.RawMessage
	out    []byte
}

/* NewCollect constructs an empty collection. */
func NewCollect() *CollectServer { return &CollectServer{} }

/* Write admits complete JSON values and flushes after admitting this call's inputs. */
func (server *CollectServer) Write(ctx context.Context, call Collect_write) error {
	server.out = nil
	values, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "collect: values", err))
	}
	for index := range values.Len() {
		value, err := values.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "collect: value", err))
		}

		if len(value) == 0 {
			continue
		}

		if !json.Valid(value) {
			return errnie.Error(errnie.Err(errnie.Validation, "collect: invalid JSON value", nil))
		}
		server.values = append(server.values, bytes.Clone(value))
	}

	if !call.Args().Flush() || len(server.values) == 0 {
		return nil
	}
	server.out, err = json.Marshal(server.values)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "collect: encode collection", err))
	}
	server.values = nil
	return nil
}

/* Done transfers a completed collection once. */
func (server *CollectServer) Done(ctx context.Context, call Collect_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "collect: results", err))
	}

	if len(server.out) == 0 {
		results.SetIdle()
		return nil
	}

	if err := results.SetOut(server.out); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "collect: output", err))
	}
	server.out = nil
	return nil
}
