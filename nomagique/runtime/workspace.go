package runtime

import (
	"context"
	"io"
	"sync/atomic"

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
/*
Ring is one disruptor handler group. Its nodes run concurrently against the
same tick, and the next ring does not start until every node here has
returned. Group is the name the diagnostics page uses for the ring.
*/
type Ring struct {
	Group string
	Nodes []Node
}

type Workspace struct {
	*System
	channel  disruptor.Disruptor
	mask     int64
	capacity int
	stages   [][]*Consumer
	buffer   []*data.Value
	onTrace  atomic.Value // func(*data.Value)
}

/*
NewWorkspace builds the disruptor pipeline. Each sink (store, hub, ...) runs in
a final handler group after every stage, receiving the tick's Value through
io.Copy(sink, value).
*/
func NewWorkspace(
	ctx context.Context,
	label string,
	stages []Ring,
	sinks ...io.Writer,
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

	for stageIdx, stage := range stages {
		consumers := make([]*Consumer, 0, len(stage.Nodes))
		group := make([]disruptor.Handler, 0, len(stage.Nodes))

		for _, node := range stage.Nodes {
			consumer := NewConsumer(node, capacity, mask)
			consumers = append(consumers, consumer)
			group = append(group, &stageHandler{
				consumer:  consumer,
				workspace: workspace,
			})
		}

		workspace.stages[stageIdx] = consumers

		if len(group) > 0 {
			opts = append(opts, disruptor.Options.NewHandlerGroup(group...))
		}
	}

	sinkGroup := make([]disruptor.Handler, 0, len(sinks))

	for _, sink := range sinks {
		if sink != nil {
			sinkGroup = append(sinkGroup, &sinkHandler{
				sink:      sink,
				workspace: workspace,
			})
		}
	}

	if len(sinkGroup) > 0 {
		opts = append(
			opts, disruptor.Options.NewHandlerGroup(sinkGroup...),
		)
	}

	// After every sink has returned, one handler hands the tick to Publish.
	opts = append(
		opts,
		disruptor.Options.NewHandlerGroup(&traceHandler{workspace: workspace}),
	)

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

/*
Publish registers the function called with each tick once every stage and
sink has finished. The diagnostics hub uses it to broadcast timings without
writing them into the stored tick. A nil function clears the hook.
*/
func (workspace *Workspace) Publish(fn func(*data.Value)) {
	if fn == nil {
		workspace.onTrace.Store(func(*data.Value) {})
		return
	}

	workspace.onTrace.Store(fn)
}

type stageHandler struct {
	consumer  *Consumer
	workspace *Workspace
}

func (sh *stageHandler) Handle(lower, upper int64) {
	for seq := lower; seq <= upper; seq++ {
		value := sh.workspace.buffer[seq&sh.workspace.mask]

		if value == nil {
			continue
		}

		sh.consumer.Step(value)
	}
}

type sinkHandler struct {
	sink      io.Writer
	workspace *Workspace
}

func (sh *sinkHandler) Handle(lower, upper int64) {
	for seq := lower; seq <= upper; seq++ {
		value := sh.workspace.buffer[seq&sh.workspace.mask]

		if value == nil {
			continue
		}

		if _, err := io.Copy(sh.sink, value); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO, "[workspace] sink copy failed", err,
			))
		}
	}
}

type traceHandler struct {
	workspace *Workspace
}

func (handler *traceHandler) Handle(lower, upper int64) {
	stored := handler.workspace.onTrace.Load()
	fn, _ := stored.(func(*data.Value))

	if fn == nil {
		return
	}

	for seq := lower; seq <= upper; seq++ {
		value := handler.workspace.buffer[seq&handler.workspace.mask]

		if value == nil {
			continue
		}

		fn(value)
	}
}
