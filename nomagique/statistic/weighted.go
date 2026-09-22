package statistic

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
WeightedServer summarises observations that did not all carry the same
authority.

Weighting by the evidence behind each value is what keeps a reading formed
from two overlapping returns from counting as much as one formed from two
hundred. The effective count reports how many equally weighted observations
the set is actually worth, which is usually fewer than how many arrived.
*/
type WeightedServer struct {
	*runtime.System
	mean      float64
	variance  float64
	total     float64
	count     float64
	effective float64
}

func NewWeighted(ctx context.Context) *WeightedServer {
	server := &WeightedServer{
		System: runtime.NewSystem(ctx, "statistic.weighted"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write summarises the observations and the authority behind each.
*/
func (server *WeightedServer) Write(ctx context.Context, call Weighted_write) error {
	values, err := call.Args().Value()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[statistic.weighted.Write] failed to read value argument",
			err,
		))
	}

	weights, err := call.Args().Weight()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[statistic.weighted.Write] failed to read weight argument",
			err,
		))
	}

	if values.Len() != weights.Len() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"statistic.weighted: every value needs the authority behind it",
			nil,
		))
	}

	server.summarise(values, weights)
	return nil
}

/*
Done reports the summary and clears it.
*/
func (server *WeightedServer) Done(ctx context.Context, call Weighted_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.weighted.Done] failed to allocate results",
			err,
		))
	}

	results.SetMean(server.mean)
	results.SetVariance(server.variance)
	results.SetTotal(server.total)
	results.SetCount(server.count)
	results.SetEffective(server.effective)

	server.mean, server.variance = 0, 0
	server.total, server.count, server.effective = 0, 0, 0

	return nil
}

/*
summarise forms the weighted mean, its spread, and how many equally weighted
observations the set is worth.
*/
func (server *WeightedServer) summarise(values, weights capnp.Float64List) {
	server.count = float64(values.Len())

	squared := 0.0

	for index := range weights.Len() {
		weight := weights.At(index)
		server.total += weight
		squared += weight * weight
	}

	// Kish: a set of unequal weights is worth this many equal ones.
	server.effective = (server.total * server.total) / squared

	for index := range values.Len() {
		server.mean += weights.At(index) * values.At(index)
	}

	server.mean /= server.total

	for index := range values.Len() {
		deviation := values.At(index) - server.mean
		server.variance += weights.At(index) * deviation * deviation
	}

	server.variance /= server.total
}
