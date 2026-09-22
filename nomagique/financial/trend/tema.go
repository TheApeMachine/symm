package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
TemaServer calculates the Triple Exponential Moving Average (TEMA).
*/
type TemaServer struct {
	*runtime.System
	calculator *trend.Tema[float64]
	value chan float64
	out <-chan float64
	result float64
	count int
}

func NewTema(ctx context.Context) *TemaServer {
	value := make(chan float64, 1)
	calculator := trend.NewTema[float64]()

	server := &TemaServer{
		System: runtime.NewSystem(ctx, "financial.trend.tema"),
		calculator: calculator,
		value: value,
		out: calculator.ComputeWithContext(ctx, value),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *TemaServer) Write(ctx context.Context, call Tema_write) error {
	valueVal := call.Args().Value()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.value <- valueVal:
	}

	server.count++

	if server.count > server.calculator.IdlePeriod() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res, ok := <-server.out:
			if !ok {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"[financial.trend.tema.Write] calculator channel closed",
					nil,
				))
			}

			server.result = res
		}
	}

	return nil
}

/*
Done returns calculated indicator results.
*/
func (server *TemaServer) Done(ctx context.Context, call Tema_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.tema.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
