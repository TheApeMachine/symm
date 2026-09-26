package arithmetic

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type DecimalAddServer struct {
	*runtime.System
	out []byte
}

func NewDecimalAdd(ctx context.Context) *DecimalAddServer {
	return &DecimalAddServer{
		System: runtime.NewSystem(ctx, "arithmetic.decimalAdd"),
	}
}

func (server *DecimalAddServer) Write(ctx context.Context, call DecimalAdd_write) error {
	left, err := call.Args().A()

	if err != nil {
		return server.Error(errnie.Err(errnie.Validation, "arithmetic.DecimalAdd: a", err))
	}
	right, err := call.Args().B()

	if err != nil {
		return server.Error(errnie.Err(errnie.Validation, "arithmetic.DecimalAdd: b", err))
	}
	a, b, err := decimalPair(left, right)

	if err != nil {
		return server.Error(err)
	}
	server.out, err = core.WriteDecimal(a.SetScale(max(a.GetScale(), b.GetScale())).Add(b))

	if err != nil {
		return server.Error(err)
	}
	return nil
}

func (server *DecimalAddServer) Done(ctx context.Context, call DecimalAdd_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return server.Error(errnie.Err(errnie.Internal, "arithmetic.DecimalAdd: allocate result", err))
	}

	if err := results.SetOut(server.out); err != nil {
		return server.Error(errnie.Err(errnie.Internal, "arithmetic.DecimalAdd: emit", err))
	}
	server.out = nil
	return nil
}
