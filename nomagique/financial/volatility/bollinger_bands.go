package volatility

import (
	"context"

	indicator "github.com/cinar/indicator/v2/volatility"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
BollingerBandsServer calculates the Bollinger Bands.
*/
type BollingerBandsServer struct {
	*runtime.System
	calculator *indicator.BollingerBands[float64]
	close chan float64
	upperOut <-chan float64
	middleOut <-chan float64
	lowerOut <-chan float64
	upper float64
	middle float64
	lower float64
	count int
}

func NewBollingerBands(ctx context.Context) *BollingerBandsServer {
	close := make(chan float64, 1)
	calculator := indicator.NewBollingerBands[float64]()

	server := &BollingerBandsServer{
		System: runtime.NewSystem(ctx, "financial.volatility.bollinger_bands"),
		calculator: calculator,
		close: close,
	}

	server.upperOut, server.middleOut, server.lowerOut = calculator.ComputeWithContext(ctx, close)

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *BollingerBandsServer) Write(ctx context.Context, call BollingerBands_write) error {
	closeVal := call.Args().Close()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.close <- closeVal:
	}

	server.count++

	if server.count > server.calculator.IdlePeriod() {
		readFirst := false
		readSecond := false
		readThird := false

		for !readFirst || !readSecond || !readThird {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case res, ok := <-server.upperOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.volatility.bollinger_bands.Write] upper channel closed",
						nil,
					))
				}

				server.upper = res
				readFirst = true
			case res, ok := <-server.middleOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.volatility.bollinger_bands.Write] middle channel closed",
						nil,
					))
				}

				server.middle = res
				readSecond = true
			case res, ok := <-server.lowerOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.volatility.bollinger_bands.Write] lower channel closed",
						nil,
					))
				}

				server.lower = res
				readThird = true
			}
		}
	}

	return nil
}

/*
Done returns calculated indicator results.
*/
func (server *BollingerBandsServer) Done(ctx context.Context, call BollingerBands_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.volatility.bollinger_bands.Done] failed to allocate done results",
			err,
		))
	}

	results.SetUpper(server.upper)
	results.SetMiddle(server.middle)
	results.SetLower(server.lower)
	return nil
}
