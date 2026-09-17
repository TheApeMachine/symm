package runtime

import (
	"context"
	"iter"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/runtime/disruptor"
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
type Workspace struct {
	*System
	channel disruptor.Disruptor
}

func NewWorkspace(
	ctx context.Context,
	label string,
	stages [][]core.Primitive,
) *Workspace {
	workload := &Workspace{}

	opts := optionList(
		disruptor.Options.BufferCapacity(
			system.Cfg.Runtime.Workspace.Buffer,
		),
	)

	for _, stage := range stages {
		group := make([]disruptor.Handler, 0, len(stage))

		for _, node := range stage {
			group = append(group, NewConsumer(node))
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
func (workspace *Workspace) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for item := range in {
			seq := workspace.channel.Reserve(1)
			workspace.channel.Commit(seq, seq)

			if !yield(item) {
				break
			}
		}
	}
}
