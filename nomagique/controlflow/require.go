package controlflow

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

/* RequireServer enforces a predicate supplied by the graph. */
type RequireServer struct{ out []byte }

/* NewRequire constructs an idle precondition primitive. */
func NewRequire() *RequireServer { return &RequireServer{} }

/* Write fails the evaluation when the supplied predicate is false. */
func (server *RequireServer) Write(ctx context.Context, call Require_write) error {
	server.out = nil
	reason, err := call.Args().Reason()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "require: reason", err))
	}

	if reason == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "require: a precondition reason is required", nil))
	}

	if !call.Args().Test() {
		return errnie.Error(errnie.Err(errnie.Validation, reason, nil))
	}
	payload, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "require: data", err))
	}
	server.out = bytes.Clone(payload)
	return nil
}

/* Done emits accepted data and releases evaluation state. */
func (server *RequireServer) Done(ctx context.Context, call Require_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "require: results", err))
	}

	if err := results.SetOut(server.out); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "require: output", err))
	}
	server.out = nil
	return nil
}
