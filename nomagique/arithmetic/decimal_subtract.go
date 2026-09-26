package arithmetic

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type DecimalSubtractServer struct {
	*runtime.System
	out []byte
}

func NewDecimalSubtract(ctx context.Context) *DecimalSubtractServer {
	return &DecimalSubtractServer{
		System: runtime.NewSystem(ctx, "arithmetic.decimalSubtract"),
	}
}

func (server *DecimalSubtractServer) Write(ctx context.Context, call DecimalSubtract_write) error {
	left, err := call.Args().A()

	if err != nil {
		return server.Error(errnie.Err(errnie.Validation, "arithmetic.DecimalSubtract: a", err))
	}
	right, err := call.Args().B()

	if err != nil {
		return server.Error(errnie.Err(errnie.Validation, "arithmetic.DecimalSubtract: b", err))
	}
	a, b, err := decimalPair(left, right)

	if err != nil {
		return server.Error(err)
	}
	server.out, err = core.WriteDecimal(a.SetScale(max(a.GetScale(), b.GetScale())).Sub(b))

	if err != nil {
		return server.Error(err)
	}
	return nil
}

func (server *DecimalSubtractServer) Done(ctx context.Context, call DecimalSubtract_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return server.Error(errnie.Err(errnie.Internal, "arithmetic.DecimalSubtract: allocate result", err))
	}

	if err := results.SetOut(server.out); err != nil {
		return server.Error(errnie.Err(errnie.Internal, "arithmetic.DecimalSubtract: emit", err))
	}
	server.out = nil
	return nil
}
