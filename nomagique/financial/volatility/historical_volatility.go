package volatility

import (
	"context"

	indicator "github.com/cinar/indicator/v2/volatility"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
HistoricalVolatilityServer calculates the Historical Volatility (HV).
*/
type HistoricalVolatilityServer struct {
	*runtime.System
	calculator *indicator.HistoricalVolatility[float64]
	price chan float64
	out <-chan float64
	result float64
	count int
}

func NewHistoricalVolatility(ctx context.Context) *HistoricalVolatilityServer {
	price := make(chan float64, 1)
	calculator := indicator.NewHistoricalVolatility[float64]()

	server := &HistoricalVolatilityServer{
		System: runtime.NewSystem(ctx, "financial.volatility.historical_volatility"),
		calculator: calculator,
		price: price,
		out: calculator.ComputeWithContext(ctx, price),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *HistoricalVolatilityServer) Write(ctx context.Context, call HistoricalVolatility_write) error {
	priceVal := call.Args().Price()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.price <- priceVal:
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
					"[financial.volatility.historical_volatility.Write] calculator channel closed",
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
func (server *HistoricalVolatilityServer) Done(ctx context.Context, call HistoricalVolatility_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.volatility.historical_volatility.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
