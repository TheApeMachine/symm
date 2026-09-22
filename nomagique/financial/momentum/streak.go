package momentum

import (
	"context"

	indicator "github.com/cinar/indicator/v2/momentum"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
StreakServer calculates the price streak.
*/
type StreakServer struct {
	*runtime.System
	calculator *indicator.Streak[float64]
	close      chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewStreak(ctx context.Context) *StreakServer {
	close := make(chan float64, 1)
	calculator := indicator.NewStreak[float64]()

	server := &StreakServer{
		System:     runtime.NewSystem(ctx, "financial.momentum.streak"),
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
func (server *StreakServer) Write(ctx context.Context, call Streak_write) error {
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
					"[financial.momentum.streak.Write] calculator channel closed",
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
func (server *StreakServer) Done(ctx context.Context, call Streak_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.momentum.streak.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
