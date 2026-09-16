package runtime

import (
	"context"

	"github.com/smarty/go-disruptor"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/system"
)

func optionList[O any](initial ...O) []O {
	return initial
}

/*
Workspace runs registered nodes in dependency stages. Nodes in one stage may
execute concurrently and query only peers owned by earlier stages. Step admits
work while READY; each consumer writes its sequence-owned result and immediately
pushes it to the Tees.
*/
type Workspace[T any] struct {
	*System
	channel  disruptor.Disruptor
	register *store.Register[T]
}

func NewWorkspace[T any](
	ctx context.Context,
	label string,
	stages [][]Node[T],
	tees ...Tee,
) *Workspace[T] {
	workload := &Workspace[T]{
		register: store.NewRegister[T](int(system.Cfg.Runtime.Workspace.Buffer)),
	}

	opts := optionList(
		disruptor.Options.BufferCapacity(
			system.Cfg.Runtime.Workspace.Buffer,
		),
	)

	peerLimit := 0

	for _, stage := range stages {
		group := make([]disruptor.Handler, len(stage))
		stageLimit := peerLimit

		for index, node := range stage {
			consumer := NewConsumer(node, workload.register, tees...)
			group[index] = consumer.SetPeerLimit(stageLimit)
			peerLimit = consumer.Identity() + 1
		}

		if len(group) > 0 {
			opts = append(opts, disruptor.Options.NewHandlerGroup(group...))
		}
	}

	channel, err := disruptor.New(opts...)
	workload.System = NewSystem(ctx, label, channel, workload)

	if err != nil {
		workload.Error(errnie.Err(
			errnie.Internal,
			"[workspace] error creating LMAX disruptor channel",
			err,
		))

		return nil
	}

	workload.channel = channel

	go workload.channel.Listen()
	return workload
}

// Step commits work without waiting for completion. The Disruptor's capacity
// barrier prevents reuse until all stages, including each consumer's Tee calls, finish.
func (workspace *Workspace[T]) Step(payload T) T {
	if workspace.Status() != READY {
		errnie.Warn(workspace.Name() + ": Step called before READY; dropping event")
		return payload
	}

	select {
	case <-workspace.Context().Done():
		workspace.Error(workspace.Context().Err())
		return payload
	default:
		if workspace.Error() != nil {
			workspace.Error(errnie.Err(
				errnie.Internal,
				"[workspace] internal error encountered",
				nil,
			))
			return payload
		}
	}

	seq := workspace.channel.Reserve(1)
	workspace.channel.Commit(seq, seq)

	return payload
}
