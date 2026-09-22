package statistic

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
RSquaredServer calculates online coefficient of determination R^2 between x and y.
*/
type RSquaredServer struct {
	*runtime.System
	count    float64
	meanX    float64
	meanY    float64
	m2X      float64
	m2Y      float64
	coMoment float64
	result   float64
}

func NewRSquared(ctx context.Context) *RSquaredServer {
	server := &RSquaredServer{
		System: runtime.NewSystem(ctx, "statistic.r_squared"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *RSquaredServer) Write(ctx context.Context, call RSquared_write) error {
	xVal := call.Args().X()
	yVal := call.Args().Y()
	server.count++
	deltaX := xVal - server.meanX
	server.meanX += deltaX / server.count
	deltaY := yVal - server.meanY
	server.meanY += deltaY / server.count
	server.m2X += deltaX * (xVal - server.meanX)
	server.m2Y += deltaY * (yVal - server.meanY)
	server.coMoment += deltaX * (yVal - server.meanY)

	if server.count > 1 {
		if server.m2X > 0 && server.m2Y > 0 {
			r := server.coMoment / (math.Sqrt(server.m2X) * math.Sqrt(server.m2Y))
			server.result = r * r
		}
	}
	return nil
}

/*
Done returns calculated results.
*/
func (server *RSquaredServer) Done(ctx context.Context, call RSquared_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.r_squared.Done] failed to allocate done results",
			err,
		))
	}

	results.SetRSquared(server.result)
	return nil
}
