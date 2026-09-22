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
	contextBytes, err := call.Args().ContextBytes()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[cognition.attractor.Write] failed to read context argument",
			err,
		))
	}

	server.class = nil
	server.prob = 0
	server.count = 0

	if len(contextBytes) == 0 {
		return nil
	}

	memory := call.Args().Memory()

	// A node that reads what was learned has to be told where the learning
	// is. Walking a structure of its own would answer every question with
	// silence while the system ran perfectly.
	if !memory.IsValid() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[cognition.attractor.Write] no learned memory is wired to this node",
			nil,
		))
	}

	prefix := BasinPrefixOf(basinOf(contextBytes))

	future, release := memory.Basin(ctx, func(params Memory_basin_Params) error {
		return params.SetPrefix(prefix)
	})
	defer release()

	basin, err := future.Struct()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[cognition.attractor.Write] failed to read the basin",
			err,
		))
	}

	classes, err := basin.Classes()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[cognition.attractor.Write] failed to read the observed classes",
			err,
		))
	}

	weights, err := basin.Weights()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[cognition.attractor.Write] failed to read what each class carries",
			err,
		))
	}

	var winner []byte
	var winning, total uint64

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

		if weight > winning {
			winning = weight
			winner = bytes.Clone(class)
		}
	}

	if total == 0 {
		return nil
	}

	server.class = winner
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

/*
basinOf reads the context out of a basin key, so a key emitted by a writer can
be handed straight back as the context to look up.
*/
func basinOf(contextBytes []byte) []byte {
	if !bytes.HasPrefix(contextBytes, []byte("b/")) {
		return contextBytes
	}

	trimmed := contextBytes[2:]
	cut := bytes.LastIndexByte(trimmed, '/')

	if cut < 0 {
		return trimmed
	}

	return trimmed[:cut]
}
