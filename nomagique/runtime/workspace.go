package runtime

import (
	"context"
	"io"
	"sync/atomic"
	"time"

	"github.com/smarty/go-disruptor"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/system"
)

// monoBase is the process clock every stage duration is measured against.
// time.Since uses the monotonic clock, so a wall-clock step cannot turn a
// short stage into a multi-second one.
var monoBase = time.Now()

func mono() int64 {
	return int64(time.Since(monoBase))
}

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
	nodes    int
	// published is the last sequence committed. A handler's backlog is how
	// far this has run ahead of the sequence it is working on.
	published atomic.Int64
	onTrace   atomic.Value // func(*data.Value)
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
		name := stage.Group

		if name == "" {
			name = "stage"
		}

		for _, node := range stage.Nodes {
			consumer := NewConsumer(node, capacity, mask)
			consumers = append(consumers, consumer)
			group = append(group, &stageHandler{
				consumer:  consumer,
				workspace: workspace,
				stageIdx:  stageIdx,
				group:     name,
			})
		}

		workspace.stages[stageIdx] = consumers
		workspace.nodes += len(consumers)

		if len(group) > 0 {
			opts = append(opts, disruptor.Options.NewHandlerGroup(group...))
		}
	}

	sinkGroup := make([]disruptor.Handler, 0, len(sinks))

	sinkStage := len(stages)

	for _, sink := range sinks {
		if sink != nil {
			sinkGroup = append(sinkGroup, &sinkHandler{
				sink:      sink,
				workspace: workspace,
				stageIdx:  sinkStage,
			})
		}
	}

	if len(sinkGroup) > 0 {
		opts = append(opts, disruptor.Options.NewHandlerGroup(sinkGroup...))
	}

	// After every sink has returned, one handler publishes the tick's spans.
	// It does no work of its own, so it is not a stage on the critical path.
	opts = append(opts, disruptor.Options.NewHandlerGroup(&traceHandler{workspace: workspace}))

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

	// At least one layer slot per node, so parallel nodes append lock-free.
	// A producer that knows a node writes more (one per trade entry) reserves
	// those slots itself before Step.
	if !value.Reserved() {
		value.Reserve(workspace.nodes)
	}

	seq := workspace.channel.Reserve(1)

	slot := seq & workspace.mask
	value.MarkEnqueued(mono())
	workspace.buffer[slot] = value

	workspace.channel.Commit(seq, seq)
	workspace.published.Store(seq)

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
	stageIdx  int
	group     string
}

func (sh *stageHandler) Handle(lower, upper int64) {
	for seq := lower; seq <= upper; seq++ {
		value := sh.workspace.buffer[seq&sh.workspace.mask]

		if value == nil {
			continue
		}

		sh.workspace.measure(value, sh.consumer.node.Name(), sh.group, sh.stageIdx, seq, func() {
			sh.consumer.Step(value)
		})
	}
}

type sinkHandler struct {
	sink      io.Writer
	workspace *Workspace
	stageIdx  int
}

func (sh *sinkHandler) Handle(lower, upper int64) {
	for seq := lower; seq <= upper; seq++ {
		value := sh.workspace.buffer[seq&sh.workspace.mask]

		if value == nil {
			continue
		}

		sh.workspace.measure(value, writerName(sh.sink), "sink", sh.stageIdx, seq, func() {
			if _, err := io.Copy(sh.sink, value); err != nil {
				errnie.Error(errnie.Err(
					errnie.IO, "[workspace] sink copy failed", err,
				))
			}
		})
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

/*
measure times one stage. Wait is how long it sat after the previous barrier
(or, for the first stage, after the tick was committed). Work is the body.
Backlog is how many newer sequences were already committed when it finished,
which is the ring running ahead of this stage.
*/
func (workspace *Workspace) measure(
	value *data.Value,
	name, group string,
	stage int,
	seq int64,
	body func(),
) {
	start := mono()
	wait := int64(0)

	if stage == 0 {
		if enqueued := value.Enqueued(); enqueued > 0 && start >= enqueued {
			wait = start - enqueued
		}
	} else if prev := value.StageDone(stage - 1); prev > 0 && start >= prev {
		wait = start - prev
	}

	body()

	end := mono()
	backlog := workspace.published.Load() - seq

	if backlog < 0 {
		backlog = 0
	}

	value.Note(data.Span{
		Name:    name,
		Group:   group,
		Stage:   stage,
		WaitNs:  wait,
		WorkNs:  end - start,
		Backlog: backlog,
	})
	value.MarkStageDone(stage, end)
}

func writerName(writer io.Writer) string {
	if named, ok := writer.(interface{ Name() string }); ok {
		if name := named.Name(); name != "" {
			return name
		}
	}

	return "sink"
}
