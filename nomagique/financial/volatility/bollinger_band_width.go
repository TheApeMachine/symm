package volatility

import (
	"context"

	indicator "github.com/cinar/indicator/v2/volatility"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
BollingerBandWidthServer calculates the Bollinger Band Width.
*/
type BollingerBandWidthServer struct {
	*runtime.System
	calculator *indicator.BollingerBandWidth[float64]
	close      chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewBollingerBandWidth(ctx context.Context) *BollingerBandWidthServer {
	close := make(chan float64, 1)
	calculator := indicator.NewBollingerBandWidth[float64]()

	server := &BollingerBandWidthServer{
		System:     runtime.NewSystem(ctx, "financial.volatility.bollinger_band_width"),
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
func (server *BollingerBandWidthServer) Write(ctx context.Context, call BollingerBandWidth_write) error {
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
					"[financial.volatility.bollinger_band_width.Write] calculator channel closed",
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
func (server *BollingerBandWidthServer) Done(ctx context.Context, call BollingerBandWidth_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.volatility.bollinger_band_width.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
