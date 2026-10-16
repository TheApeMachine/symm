package runtime

import (
	"context"

	"github.com/smarty/go-disruptor"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/system"
)

func optionList[O any](initial ...O) []O {
	return initial
}

/*
Workspace runs registered nodes in dependency stages using an LMAX Disruptor.
Between stages, a post-barrier runtime join handler composes completed prior
producer outputs into an immutable WORM join measurement without metric copying.
*/
type Workspace struct {
	*System
	channel  disruptor.Disruptor
	mask     int64
	capacity int
	stages   [][]*Consumer
	buffer   []*data.Value
}

func NewWorkspace(
	ctx context.Context,
	label string,
	stages [][]Node,
) *Workspace {
	capacity := 8192

	if system.Cfg.Runtime.Workspace.Buffer > 0 {
		capacity = int(system.Cfg.Runtime.Workspace.Buffer)
	}

	mask := int64(capacity - 1)

	workspace := &Workspace{
		mask:     mask,
		capacity: capacity,
		stages:   make([][]*Consumer, len(stages)),
		buffer:   make([]*data.Value, capacity),
	}

	opts := optionList(
		disruptor.Options.BufferCapacity(uint32(capacity)),
		disruptor.Options.WriterCount(1),
	)

	for stageIdx, stageNodes := range stages {
		consumers := make([]*Consumer, 0, len(stageNodes))
		group := make([]disruptor.Handler, 0, len(stageNodes))

		for _, node := range stageNodes {
			consumer := NewConsumer(node, capacity, mask)
			consumers = append(consumers, consumer)
			group = append(group, &stageHandler{
				consumer:  consumer,
				workspace: workspace,
				stageIdx:  stageIdx,
			})
		}

		workspace.stages[stageIdx] = consumers

		if len(group) > 0 {
			opts = append(opts, disruptor.Options.NewHandlerGroup(group...))
		}
	}

	channel, err := disruptor.New(opts...)
	workspace.System = NewSystem(ctx, label, channel, workspace)

	if err != nil {
		workspace.Error(errnie.Err(
			errnie.Internal,
			"[workspace] error creating LMAX disruptor channel",
			err,
		))
		return nil
	}

	workspace.channel = channel
	go workspace.channel.Listen()
	return workspace
}

func (workspace *Workspace) Step(value *data.Value) *data.Value {
	if workspace.Status() != READY {
		errnie.Warn(workspace.Name() + ": Step called before READY; dropping event")
		return value
	}

	if value == nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"[workspace] nil payload passed to Step",
			nil,
		))

		return value
	}

	select {
	case <-workspace.Context().Done():
		workspace.Error(workspace.Context().Err())
		return value
	default:
		if workspace.Error() != nil {
			workspace.Error(errnie.Err(
				errnie.Internal,
				"[workspace] internal error encountered",
				nil,
			))
			return value
		}
	}

	seq := workspace.channel.Reserve(1)

	slot := seq & workspace.mask
	workspace.buffer[slot] = value

	workspace.channel.Commit(seq, seq)

	return value
}

type stageHandler struct {
	consumer  *Consumer
	workspace *Workspace
	stageIdx  int
}

func (sh *stageHandler) Handle(lower, upper int64) {
	for seq := lower; seq <= upper; seq++ {
		slot := seq & sh.workspace.mask
		sh.consumer.Step(sh.workspace.buffer[slot])
	}
}
