package arithmetic

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type DecimalMultiplyServer struct {
	*runtime.System
	out []byte
}

func NewDecimalMultiply(ctx context.Context) *DecimalMultiplyServer {
	return &DecimalMultiplyServer{
		System: runtime.NewSystem(ctx, "arithmetic.decimalMultiply"),
	}
}

func (server *DecimalMultiplyServer) Write(ctx context.Context, call DecimalMultiply_write) error {
	left, err := call.Args().A()

	if err != nil {
		return server.Error(errnie.Err(errnie.Validation, "arithmetic.DecimalMultiply: a", err))
	}
	right, err := call.Args().B()

	if err != nil {
		return server.Error(errnie.Err(errnie.Validation, "arithmetic.DecimalMultiply: b", err))
	}
	a, b, err := decimalPair(left, right)

	if err != nil {
		return server.Error(err)
	}
	server.out, err = core.WriteDecimal(a.SetScale(a.GetScale() + b.GetScale()).Mul(b))

	if err != nil {
		return server.Error(err)
	}
	return nil
}

func (server *DecimalMultiplyServer) Done(ctx context.Context, call DecimalMultiply_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return server.Error(errnie.Err(errnie.Internal, "arithmetic.DecimalMultiply: allocate result", err))
	}

	if err := results.SetOut(server.out); err != nil {
		return server.Error(errnie.Err(errnie.Internal, "arithmetic.DecimalMultiply: emit", err))
	}
	server.out = nil
	return nil
}
