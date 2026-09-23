package arithmetic

import (
	"context"
	"math/big"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type DecimalQuotientServer struct {
	*runtime.System
	out []byte
}

func NewDecimalQuotient(ctx context.Context) *DecimalQuotientServer {
	return &DecimalQuotientServer{
		System: runtime.NewSystem(ctx, "arithmetic.decimalQuotient"),
	}
}

func (server *DecimalQuotientServer) Write(ctx context.Context, call DecimalQuotient_write) error {
	left, err := call.Args().A()

	if err != nil {
		return server.Error(errnie.Err(errnie.Validation, "arithmetic.DecimalQuotient: a", err))
	}
	right, err := call.Args().B()

	if err != nil {
		return server.Error(errnie.Err(errnie.Validation, "arithmetic.DecimalQuotient: b", err))
	}
	a, b, err := decimalPair(left, right)

	if err != nil {
		return server.Error(err)
	}
	if b.Sign() == 0 {
		return server.Error(errnie.Err(errnie.Validation, "arithmetic.DecimalQuotient: division by zero", nil))
	}
	unit := new(big.Int).Exp(big.NewInt(10), new(big.Int).SetUint64(call.Args().Places()), nil)
	scaled := new(big.Rat).Mul(new(big.Rat).Quo(a, b), new(big.Rat).SetInt(unit))
	floor := new(big.Int).Div(scaled.Num(), scaled.Denom())
	server.out, err = core.WriteDecimal(new(big.Rat).SetFrac(floor, unit))

	if err != nil {
		return server.Error(err)
	}
	return nil
}

func (server *DecimalQuotientServer) Done(ctx context.Context, call DecimalQuotient_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return server.Error(errnie.Err(errnie.Internal, "arithmetic.DecimalQuotient: allocate result", err))
	}

	if err := results.SetOut(server.out); err != nil {
		return server.Error(errnie.Err(errnie.Internal, "arithmetic.DecimalQuotient: emit", err))
	}
	server.out = nil
	return nil
}
