package hawkes

import (
	"context"

	"github.com/theapemachine/errnie"
)

/*
EventsServer retains one observed realisation of a multivariate point
process: when each arrival happened and which component it landed on. It is
the only node in this package that remembers the process at all, which is
what leaves every other node a pure function of the window it is handed.

Arrivals are retained in the order they occur. An arrival that predates the
one before it is refused rather than inserted: the exponential kernel is
accumulated by walking forward in time, and a retrospective insertion would
invalidate every excitation already carried past it.

The oldest retained arrival marks the window's origin and is prehistory. It
still excites what follows but is not itself a counted observation, because
the process before it was not observed and its own intensity is therefore
unknown.
*/
type EventsServer struct {
	times      []float64
	components []float64
	dimension  int
	started    bool
}

func NewEvents() *EventsServer {
	return &EventsServer{}
}

func (server *EventsServer) Write(ctx context.Context, call Events_write) error {
	args := call.Args()
	time := args.Time()
	component := int(args.Component())
	dimension := int(args.Dimension())
	capacity := int(args.Capacity())

	if dimension > 0 {
		server.dimension = dimension
	}

	if server.dimension <= 0 || component < 0 || component >= server.dimension {
		return nil
	}

	if server.started && time < server.times[len(server.times)-1] {
		return nil
	}

	server.times = append(server.times, time)
	server.components = append(server.components, float64(component))
	server.started = true

	if capacity <= 0 || len(server.times) <= capacity {
		return nil
	}

	server.times = server.times[len(server.times)-capacity:]
	server.components = server.components[len(server.components)-capacity:]
	return nil
}

func (server *EventsServer) Done(ctx context.Context, call Events_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes events: failed to allocate results",
			err,
		))
	}

	timesList, err := results.NewTimes(int32(len(server.times)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes events: failed to allocate times list",
			err,
		))
	}

	for index, value := range server.times {
		timesList.Set(index, value)
	}

	componentsList, err := results.NewComponents(int32(len(server.components)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes events: failed to allocate components list",
			err,
		))
	}

	for index, value := range server.components {
		componentsList.Set(index, value)
	}

	counts := make([]float64, server.dimension)

	for _, component := range server.components {
		counts[int(component)]++
	}

	countsList, err := results.NewCounts(int32(len(counts)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes events: failed to allocate counts list",
			err,
		))
	}

	for index, value := range counts {
		countsList.Set(index, value)
	}

	results.SetCount(float64(len(server.times)))

	if len(server.times) == 0 {
		return nil
	}

	origin := server.times[0]
	horizon := server.times[len(server.times)-1]
	results.SetOrigin(origin)
	results.SetHorizon(horizon)
	results.SetSpan(horizon - origin)
	return nil
}
