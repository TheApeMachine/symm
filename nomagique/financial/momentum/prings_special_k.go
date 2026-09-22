package momentum

import (
	"context"

	indicator "github.com/cinar/indicator/v2/momentum"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
PringsSpecialKServer calculates the Pring's Special K.
*/
type PringsSpecialKServer struct {
	*runtime.System
	calculator *indicator.PringsSpecialK[float64]
	close      chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewPringsSpecialK(ctx context.Context) *PringsSpecialKServer {
	close := make(chan float64, 1)
	calculator := indicator.NewPringsSpecialK[float64]()

	server := &PringsSpecialKServer{
		System:     runtime.NewSystem(ctx, "financial.momentum.prings_special_k"),
		calculator: calculator,
		close:      close,
		out:        calculator.ComputeWithContext(ctx, close),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *PringsSpecialKServer) Write(ctx context.Context, call PringsSpecialK_write) error {
	closeVal := call.Args().Close()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.close <- closeVal:
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
					"[financial.momentum.prings_special_k.Write] calculator channel closed",
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
func (server *PringsSpecialKServer) Done(ctx context.Context, call PringsSpecialK_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.momentum.prings_special_k.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
