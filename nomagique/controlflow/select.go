package controlflow

import (
	"bytes"
	capnp "capnproto.org/go/capnp/v3"
	"context"

	"github.com/theapemachine/errnie"
)

/* SelectServer selects one supplied value without retaining either branch. */
type SelectServer struct{ out []byte }

func NewSelect() *SelectServer { return &SelectServer{} }

/* Write selects one supplied arrival; the unselected branch cannot substitute. */
func (server *SelectServer) Write(ctx context.Context, call Select_write) error {
	var arrivals capnp.DataList
	var err error

	if !call.Args().Test() {
		arrivals, err = call.Args().No()
	}

	if call.Args().Test() {
		arrivals, err = call.Args().Yes()
	}

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "select: read selected branch", err))
	}

	server.out = nil

	for index := range arrivals.Len() {
		value, err := arrivals.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "select: read branch arrival", err))
		}

		if len(value) == 0 {
			continue
		}

		if len(server.out) != 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "select: selected branch has multiple arrivals", nil))
		}
		server.out = bytes.Clone(value)
	}
	return nil
}

/* Done emits the selected bytes and clears the evaluation. */
func (server *SelectServer) Done(ctx context.Context, call Select_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "select: allocate result", err))
	}

	if len(server.out) == 0 {
		results.SetAbsent()
		return nil
	}

	if err := results.SetOut(server.out); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "select: emit result", err))
	}

	server.out = nil
	return nil
}
