package statistic

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
SkewServer calculates online sample skewness of streaming observations.
*/
type SkewServer struct {
	*runtime.System
	count float64
	mean float64
	m2 float64
	m3 float64
	result float64
}

func NewSkew(ctx context.Context) *SkewServer {
	server := &SkewServer{
		System: runtime.NewSystem(ctx, "statistic.skew"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *SkewServer) Write(ctx context.Context, call Skew_write) error {
	valueVal := call.Args().Value()
	server.count++

	if server.count == 1 {
		server.mean = valueVal
		return nil
	}

	delta := valueVal - server.mean
	deltaN := delta / server.count
	term1 := delta * deltaN * (server.count - 1)
	server.mean += deltaN
	server.m3 += term1*deltaN*(server.count-2) - 3*deltaN*server.m2
	server.m2 += term1

	if server.count > 2 && server.m2 > 0 {
		variance := server.m2 / (server.count - 1)
		stdDev := math.Sqrt(variance)
		server.result = (server.m3 / server.count) / (stdDev * stdDev * stdDev)
	}
	return nil
}

/*
Done returns calculated results.
*/
func (server *SkewServer) Done(ctx context.Context, call Skew_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.skew.Done] failed to allocate done results",
			err,
		))
	}

	results.SetSkew(server.result)
	return nil
}
