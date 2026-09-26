package arithmetic

import (
	"context"
	"math"

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

	if call.Args().Places() > math.MaxInt64 {
		return server.Error(errnie.Err(errnie.Validation, "decimal quotient: precision exceeds representation", nil))
	}
	places := int64(call.Args().Places())
	scale := max(places, a.GetScale(), b.GetScale())
	server.out, err = core.WriteDecimal(a.SetScale(scale).Div(b).SetScale(places))

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
