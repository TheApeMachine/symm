package momentum

import (
	"context"

	indicator "github.com/cinar/indicator/v2/momentum"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
TdSequentialServer calculates the TD Sequential indicator.
*/
type TdSequentialServer struct {
	*runtime.System
	calculator       *indicator.TdSequential[float64]
	close            chan float64
	buySetupOut      <-chan float64
	buySetup         float64
	sellSetupOut     <-chan float64
	sellSetup        float64
	buyCountdownOut  <-chan float64
	buyCountdown     float64
	sellCountdownOut <-chan float64
	sellCountdown    float64
	count            int
}

func NewTdSequential(ctx context.Context) *TdSequentialServer {
	close := make(chan float64, 1)
	calculator := indicator.NewTdSequential[float64]()

	server := &TdSequentialServer{
		System:     runtime.NewSystem(ctx, "financial.momentum.td_sequential"),
		calculator: calculator,
		close:      close,
	}

	server.buySetupOut, server.sellSetupOut, server.buyCountdownOut, server.sellCountdownOut = calculator.ComputeWithContext(ctx, close)

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *TdSequentialServer) Write(ctx context.Context, call TdSequential_write) error {
	closeVal := call.Args().Close()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.close <- closeVal:
	}

	server.count++

	if server.count > server.calculator.IdlePeriod() {
		readBuySetup := false
		readSellSetup := false
		readBuyCountdown := false
		readSellCountdown := false

		for !readBuySetup || !readSellSetup || !readBuyCountdown || !readSellCountdown {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case res, ok := <-server.buySetupOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.td_sequential.Write] buySetup channel closed",
						nil,
					))
				}

				server.buySetup = res
				readBuySetup = true
			case res, ok := <-server.sellSetupOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.td_sequential.Write] sellSetup channel closed",
						nil,
					))
				}

				server.sellSetup = res
				readSellSetup = true
			case res, ok := <-server.buyCountdownOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.td_sequential.Write] buyCountdown channel closed",
						nil,
					))
				}

				server.buyCountdown = res
				readBuyCountdown = true
			case res, ok := <-server.sellCountdownOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.td_sequential.Write] sellCountdown channel closed",
						nil,
					))
				}

				server.sellCountdown = res
				readSellCountdown = true
			}
		}
	}

	return nil
}

/*
Done returns calculated indicator results.
*/
func (server *TdSequentialServer) Done(ctx context.Context, call TdSequential_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.momentum.td_sequential.Done] failed to allocate done results",
			err,
		))
	}

	results.SetBuySetup(server.buySetup)
	results.SetSellSetup(server.sellSetup)
	results.SetBuyCountdown(server.buyCountdown)
	results.SetSellCountdown(server.sellCountdown)
	return nil
}
