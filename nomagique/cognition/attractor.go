package cognition

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
AttractorServer answers what a context most strongly attracts to: of every
class observed in that basin, the one carrying the most reinforcement.

It reports the share of the basin's reinforcement the winner holds, so a
class that has always followed a context is distinguished from one that
merely followed it most often. A context never observed attracts to nothing,
and reports so rather than naming an arbitrary class.
*/
type AttractorServer struct {
	*runtime.System
	class []byte
	prob  float64
	count int64
}

func NewAttractor(ctx context.Context) *AttractorServer {
	server := &AttractorServer{
		System: runtime.NewSystem(ctx, "cognition.attractor"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write walks the basin and settles on the class carrying the most weight.
*/
func (server *AttractorServer) Write(ctx context.Context, call Attractor_write) error {
	server.class = nil
	server.prob = 0
	server.count = 0
	classes, err := call.Args().Classes()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "attractor: classes", err))
	}
	weights, err := call.Args().Weights()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "attractor: weights", err))
	}

	if classes.Len() != weights.Len() {
		return errnie.Error(errnie.Err(errnie.Validation, "attractor: class and weight lengths differ", nil))
	}
	var winner []byte
	var winning, total uint64
	tied := false

	for index := range classes.Len() {
		class, err := classes.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[cognition.attractor.Write] failed to read an observed class",
				err,
			))
		}

		weight := weights.At(index)
		total += weight
		server.count++

		if weight == winning {
			tied = true
		}

		if weight > winning {
			tied = false
			winning = weight
			winner = bytes.Clone(class)
		}
	}

	if total == 0 {
		return nil
	}

	if !tied {
		server.class = winner
	}
	server.prob = float64(winning) / float64(total)

	return nil
}

/*
Done reports the class the context attracts to and how much of the basin's
reinforcement it holds.
*/
func (server *AttractorServer) Done(ctx context.Context, call Attractor_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[cognition.attractor.Done] failed to allocate results",
			err,
		))
	}

	results.SetProb(server.prob)
	results.SetCount(server.count)

	if len(server.class) == 0 {
		return nil
	}

	if err := results.SetClass(server.class); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[cognition.attractor.Done] failed to set class",
			err,
		))
	}

	server.class = nil
	return nil
}
