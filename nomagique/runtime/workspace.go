package runtime

import (
	"context"
	goruntime "runtime"
	"sync/atomic"
	"time"

	"github.com/smarty/go-disruptor"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/system"
)

type adaptiveWaitStrategy struct{}

func (adaptiveWaitStrategy) Gate(int64) {
	goruntime.Gosched()
}

func (adaptiveWaitStrategy) Idle(count int64) {
	if count < 100 {
		goruntime.Gosched()
		return
	}

	time.Sleep(100 * time.Microsecond)
}

func (adaptiveWaitStrategy) Reserve(count int64) {
	if count < 64 {
		goruntime.Gosched()
		return
	}

	time.Sleep(50 * time.Microsecond)
}

func optionList[O any](initial ...O) []O {
	return initial
}

type Sequencer interface {
	SetSeqIdx(seq int64)
}

/*
Workspace runs registered nodes in dependency stages using an LMAX Disruptor.
Between stages, a post-barrier runtime join handler composes completed prior
producer outputs into an immutable WORM join measurement without metric copying.

For each sequence S, the architecture is:

	ingress[S]                       WORM

	stage 0:
	    correlation[S]               WORM → Tee
	    liquidity[S]                 WORM → Tee
	    cvd[S]                       WORM → Tee
	    hawkes[S]                    WORM → Tee
	    ...
	             LMAX BARRIER
	join 0:
	    runtime:join[S]              WORM (Peers: stage 0 outputs)
	             LMAX BARRIER

	stage 1:
	    category[S]                  WORM → Tee
	    resonance[S]                 WORM → Tee
	    manifold[S]                  WORM → Tee
	             LMAX BARRIER
	join 1:
	    runtime:join[S]              WORM (Peers: stage 1 outputs)
	             LMAX BARRIER

	stage 2:
	    cognition[S]                 WORM → Tee
	             LMAX BARRIER
	join 2:
	    runtime:join[S]              WORM (Peers: stage 2 outputs)
	             LMAX BARRIER

	stage 3:
	    training[S]                  WORM → Tee
*/
type Workspace struct {
	*System
	channel    disruptor.Disruptor
	buffer     []*data.Measurement[float64]
	mask       int64
	capacity   int
	sequence   atomic.Int64
	stages     [][]*Consumer
	joins      [][]*data.Measurement[float64] // [stageIdx][slot]
	joinArenas []*data.ArenaOwner
}

func NewWorkspace(
	ctx context.Context,
	writerCount uint8,
	label string,
	stages [][]Node,
	tees ...Tee,
) *Workspace {
	capacity := 1024
	if system.Cfg.Runtime.Workspace.Buffer > 0 {
		capacity = int(system.Cfg.Runtime.Workspace.Buffer)
	}
	mask := int64(capacity - 1)

	numJoins := len(stages) - 1
	if numJoins < 0 {
		numJoins = 0
	}

	workspace := &Workspace{
		buffer:     make([]*data.Measurement[float64], capacity),
		mask:       mask,
		capacity:   capacity,
		stages:     make([][]*Consumer, len(stages)),
		joins:      make([][]*data.Measurement[float64], numJoins),
		joinArenas: make([]*data.ArenaOwner, numJoins),
	}

	for joinIdx := 0; joinIdx < numJoins; joinIdx++ {
		workspace.joins[joinIdx] = make([]*data.Measurement[float64], capacity)
		workspace.joinArenas[joinIdx] = data.NewArenaOwner(capacity)
		workspace.joinArenas[joinIdx].SetWindow(capacity)
	}

	opts := optionList(
		disruptor.Options.BufferCapacity(uint32(capacity)),
		disruptor.Options.WriterCount(writerCount),
		disruptor.Options.WaitStrategy(adaptiveWaitStrategy{}),
	)

	for stageIdx, stageNodes := range stages {
		consumers := make([]*Consumer, 0, len(stageNodes))
		group := make([]disruptor.Handler, 0, len(stageNodes))

		for _, node := range stageNodes {
			consumer := NewConsumer(node, capacity, mask, tees...)
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

		// Insert post-barrier join handler between stages
		if stageIdx < numJoins {
			jh := &joinHandler{
				workspace: workspace,
				stageIdx:  stageIdx,
			}
			opts = append(opts, disruptor.Options.NewHandlerGroup(jh))
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

func (workspace *Workspace) Step(payload *data.Measurement[float64]) *data.Measurement[float64] {
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

	if payload != nil {
		payload.SetSeqIdx(workspace.sequence.Add(1))
	}

	workspace.buffer[seq&workspace.mask] = payload
	workspace.channel.Commit(seq, seq)
	return payload
}

/*
Sequence allocates and returns the next strictly monotone sequence index for
observations that bypass the LMAX Disruptor stage pipeline (e.g. raw Level-3 orders).
*/
func (workspace *Workspace) Sequence() int64 {
	if workspace == nil {
		return 1
	}

	return workspace.sequence.Add(1)
}

func (workspace *Workspace) Stages() [][]*Consumer {
	return workspace.stages
}

func (workspace *Workspace) Joins() [][]*data.Measurement[float64] {
	return workspace.joins
}

type stageHandler struct {
	consumer  *Consumer
	workspace *Workspace
	stageIdx  int
}

func (sh *stageHandler) Handle(lower, upper int64) {
	for seq := lower; seq <= upper; seq++ {
		slot := seq & sh.workspace.mask
		var prior *data.Measurement[float64]
		if sh.stageIdx == 0 {
			prior = sh.workspace.buffer[slot]
		} else {
			prior = sh.workspace.joins[sh.stageIdx-1][slot]
		}

		sh.consumer.Step(prior, seq)
	}
}

type joinHandler struct {
	workspace *Workspace
	stageIdx  int
}

func (jh *joinHandler) Handle(lower, upper int64) {
	for seq := lower; seq <= upper; seq++ {
		slot := seq & jh.workspace.mask
		ingress := jh.workspace.buffer[slot]
		joinArena := jh.workspace.joinArenas[jh.stageIdx]
		if joinArena != nil {
			joinArena.Advance(seq)
		}

		join := joinArena.NewMeasurement("runtime:join")
		if ingress != nil {
			join.Epoch = ingress.Epoch
			join.Tick = ingress.Tick
			join.Label = ingress.Label
			join.SeqIdx = ingress.SeqIdx
			join.At = ingress.At
			join.From = ingress.From
		}

		producers := jh.workspace.stages[jh.stageIdx]
		var peers []*data.Measurement[float64]

		if jh.stageIdx > 0 {
			prevJoin := jh.workspace.joins[jh.stageIdx-1][slot]

			if prevJoin != nil && len(prevJoin.Peers) > 0 {
				peers = make([]*data.Measurement[float64], 0, len(prevJoin.Peers)+len(producers))
				peers = append(peers, prevJoin.Peers...)
			}
		}

		if peers == nil {
			peers = make([]*data.Measurement[float64], 0, len(producers))
		}

		for _, consumer := range producers {
			if pub := consumer.Published(seq); pub != nil {
				peers = append(peers, pub)
			}
		}
		join.Peers = peers

		jh.workspace.joins[jh.stageIdx][slot] = join
	}
}
