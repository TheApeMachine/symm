package types

import (
	"bytes"
	"context"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
JSONServer represents a JSON data payload node in the execution graph.
It accepts JSON text (configured via the node's input text control in Flume
or streamed via Write), marshals it into canonical byte payload using sonic,
and emits it downstream as Data on Done.
*/
type JSONServer struct {
	*runtime.System
	data []byte
}

func NewJSON(ctx context.Context) *JSONServer {
	server := &JSONServer{
		System: runtime.NewSystem(ctx, "types.json"),
	}

	server.Transition(runtime.READY)
	return server
}

func NewJSONWithBytes(ctx context.Context, payload []byte) *JSONServer {
	server := &JSONServer{
		System: runtime.NewSystem(ctx, "types.json"),
		data:   bytes.Clone(payload),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write accepts inbound JSON text, parses and normalizes it via sonic.Marshal,
and retains the resulting byte payload.
*/
func (server *JSONServer) Write(ctx context.Context, call JSON_write) error {
	rawText, err := call.Args().Text()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"types.json: failed to read text arg",
			err,
		))
	}

	if len(rawText) == 0 {
		return nil
	}

	var parsed any
	unmarshalErr := sonic.UnmarshalString(rawText, &parsed)

	if unmarshalErr == nil {
		marshaled, marshalErr := sonic.Marshal(parsed)

		if marshalErr == nil {
			server.data = marshaled
			return nil
		}
	}

	server.data = []byte(rawText)
	return nil
}

/*
Done emits the JSON byte payload downstream.
*/
func (server *JSONServer) Done(ctx context.Context, call JSON_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"types.json: failed to allocate done results",
			err,
		))
	}

	if len(server.data) > 0 {
		if err := results.SetOut(server.data); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"types.json: failed to set out",
				err,
			))
		}
	}

	return nil
}

/*
Value returns the current JSON payload bytes.
*/
func (server *JSONServer) Value() []byte {
	return server.data
}

/*
Set directly updates the JSON payload bytes.
*/
func (server *JSONServer) Set(payload []byte) {
	server.data = bytes.Clone(payload)
}
