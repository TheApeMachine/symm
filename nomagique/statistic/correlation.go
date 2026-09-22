package statistic

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
CorrelationServer calculates online Pearson correlation coefficient between two streaming variables.
*/
type CorrelationServer struct {
	*runtime.System
	count float64
	meanX float64
	meanY float64
	m2X float64
	m2Y float64
	coMoment float64
	result float64
}

func NewCorrelation(ctx context.Context) *CorrelationServer {
	server := &CorrelationServer{
		System: runtime.NewSystem(ctx, "statistic.correlation"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *CorrelationServer) Write(ctx context.Context, call Correlation_write) error {
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
			server.result = server.coMoment / (math.Sqrt(server.m2X) * math.Sqrt(server.m2Y))
		}
	}
	return nil
}

/*
Done returns calculated results.
*/
func (server *CorrelationServer) Done(ctx context.Context, call Correlation_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.correlation.Done] failed to allocate done results",
			err,
		))
	}

	results.SetCorrelation(server.result)
	return nil
}
