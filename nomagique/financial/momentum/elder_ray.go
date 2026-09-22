package momentum

import (
	"context"

	indicator "github.com/cinar/indicator/v2/momentum"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
ElderRayServer calculates the Elder-Ray Index.
*/
type ElderRayServer struct {
	*runtime.System
	calculator   *indicator.ElderRay[float64]
	high         chan float64
	low          chan float64
	close        chan float64
	bullPowerOut <-chan float64
	bullPower    float64
	bearPowerOut <-chan float64
	bearPower    float64
	count        int
}

func NewElderRay(ctx context.Context) *ElderRayServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	close := make(chan float64, 1)
	calculator := indicator.NewElderRay[float64]()

	server := &ElderRayServer{
		System:     runtime.NewSystem(ctx, "financial.momentum.elder_ray"),
		calculator: calculator,
		high:       high,
		low:        low,
		close:      close,
	}

	server.bullPowerOut, server.bearPowerOut = calculator.ComputeWithContext(ctx, high, low, close)

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *ElderRayServer) Write(ctx context.Context, call ElderRay_write) error {
	highVal := call.Args().High()
	lowVal := call.Args().Low()
	closeVal := call.Args().Close()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.high <- highVal:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.low <- lowVal:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.close <- closeVal:
	}

	server.count++

	if server.count > server.calculator.IdlePeriod() {
		readBullPower := false
		readBearPower := false

		for !readBullPower || !readBearPower {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case res, ok := <-server.bullPowerOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.elder_ray.Write] bullPower channel closed",
						nil,
					))
				}

				server.bullPower = res
				readBullPower = true
			case res, ok := <-server.bearPowerOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.elder_ray.Write] bearPower channel closed",
						nil,
					))
				}

				server.bearPower = res
				readBearPower = true
			}
		}
	}

	return nil
}

/*
Done returns calculated indicator results.
*/
func (server *ElderRayServer) Done(ctx context.Context, call ElderRay_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.momentum.elder_ray.Done] failed to allocate done results",
			err,
		))
	}

	results.SetBullPower(server.bullPower)
	results.SetBearPower(server.bearPower)
	return nil
}
