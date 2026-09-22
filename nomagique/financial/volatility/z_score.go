package volatility

import (
	"context"

	indicator "github.com/cinar/indicator/v2/volatility"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
ZScoreServer calculates the Z-Score.
*/
type ZScoreServer struct {
	*runtime.System
	calculator *indicator.ZScore[float64]
	value chan float64
	out <-chan float64
	result float64
	count int
}

func NewZScore(ctx context.Context) *ZScoreServer {
	value := make(chan float64, 1)
	calculator := indicator.NewZScore[float64]()

	server := &ZScoreServer{
		System: runtime.NewSystem(ctx, "financial.volatility.z_score"),
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
func (server *ZScoreServer) Write(ctx context.Context, call ZScore_write) error {
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
					"[financial.volatility.z_score.Write] calculator channel closed",
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
func (server *ZScoreServer) Done(ctx context.Context, call ZScore_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.volatility.z_score.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
