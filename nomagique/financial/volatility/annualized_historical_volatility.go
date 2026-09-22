package volatility

import (
	"context"

	indicator "github.com/cinar/indicator/v2/volatility"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
AnnualizedHistoricalVolatilityServer calculates the Annualized Historical Volatility (AHV).
*/
type AnnualizedHistoricalVolatilityServer struct {
	*runtime.System
	calculator *indicator.AnnualizedHistoricalVolatility[float64]
	price chan float64
	out <-chan float64
	result float64
	count int
}

func NewAnnualizedHistoricalVolatility(ctx context.Context) *AnnualizedHistoricalVolatilityServer {
	price := make(chan float64, 1)
	calculator := indicator.NewAnnualizedHistoricalVolatility[float64]()

	server := &AnnualizedHistoricalVolatilityServer{
		System: runtime.NewSystem(ctx, "financial.volatility.annualized_historical_volatility"),
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
func (server *AnnualizedHistoricalVolatilityServer) Write(ctx context.Context, call AnnualizedHistoricalVolatility_write) error {
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
					"[financial.volatility.annualized_historical_volatility.Write] calculator channel closed",
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
func (server *AnnualizedHistoricalVolatilityServer) Done(ctx context.Context, call AnnualizedHistoricalVolatility_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.volatility.annualized_historical_volatility.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
