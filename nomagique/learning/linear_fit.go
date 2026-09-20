package learning

import (
	"context"

	"github.com/theapemachine/errnie"
)

type LinearFitServer struct {
	count     float64
	sumX      float64
	sumY      float64
	sumXX     float64
	sumYY     float64
	sumXY     float64
	slope     float64
	intercept float64
	r2        float64
}

func NewLinearFit() *LinearFitServer {
	return &LinearFitServer{}
}

func (server *LinearFitServer) Write(ctx context.Context, call LinearFit_write) error {
	args := call.Args()
	valX := args.X()
	valY := args.Y()

	server.count++
	server.sumX += valX
	server.sumY += valY
	server.sumXX += valX * valX
	server.sumYY += valY * valY
	server.sumXY += valX * valY

	denom := server.count*server.sumXX - server.sumX*server.sumX
	if denom != 0 {
		server.slope = (server.count*server.sumXY - server.sumX*server.sumY) / denom
		server.intercept = (server.sumY - server.slope*server.sumX) / server.count
		totalVar := server.count*server.sumYY - server.sumY*server.sumY
		if totalVar != 0 {
			server.r2 = (server.slope * server.slope * denom) / totalVar
		}
	}

	return nil
}

func (server *LinearFitServer) Done(ctx context.Context, call LinearFit_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"linear_fit: alloc results failed",
			err,
		))
	}

	results.SetSlope(server.slope)
	results.SetIntercept(server.intercept)
	results.SetR2(server.r2)

	server.slope = 0
	server.intercept = 0
	server.r2 = 0
	return nil
}
