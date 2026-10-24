package runtime

import (
	"context"
	"io"
	"sync/atomic"
	"time"

	"github.com/smarty/go-disruptor"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
	"golang.design/x/lockfree/lf"
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
	queue    *lf.Queue[*data.Value]
	channel  disruptor.Disruptor
	mask     int64
	capacity int
	stages   [][]*Consumer
	buffer   []*data.Value
	onTrace  atomic.Value
	store    *store.Client
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
) *Workspace {
	capacity := 8192

	if system.Cfg.Runtime.Workspace.Buffer > 0 {
		capacity = int(system.Cfg.Runtime.Workspace.Buffer)
	}

	mask := int64(capacity - 1)

	workspace := &Workspace{
		queue:    lf.NewQueue[*data.Value](),
		mask:     mask,
		capacity: capacity,
		stages:   make([][]*Consumer, len(stages)),
		buffer:   make([]*data.Value, capacity),
		store:    store.NewClient(ctx, system.Cfg.Storage.S3.Bucket),
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
	go workspace.drain()

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

	workspace.queue.Enqueue(value)
	return value
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

func (workspace *Workspace) drain() {
	for {
		select {
		case <-workspace.Context().Done():
			return
		default:
		}

		if workspace.queue.Length() == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		value, ok := workspace.queue.Dequeue()

		if !ok {
			continue
		}

		io.Copy(ui.UI, value)
		workspace.store.WriteAtPrefix(value.Prefix, value.Bytes())
	}
}
