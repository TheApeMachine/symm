package runtime

import (
	"context"

	"github.com/smarty/go-disruptor"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/system"
)

func optionList[O any](initial ...O) []O {
	return initial
}

type Sequencer interface {
	SetSeqIdx(seq int64)
}

/*
Workspace runs registered nodes in dependency stages using an LMAX Disruptor.
Each node's Consumer owns a preallocated ring of output Measurement slots.

For each sequence S, the architecture is:

	ingress[S]                       WORM

	stage 0:
	    correlation[S]               WORM → Tee
	    liquidity[S]                 WORM → Tee
	    cvd[S]                       WORM → Tee
	    hawkes[S]                    WORM → Tee
	    ...
	             GROUP BARRIER

	stage 1:
	    category[S]                  WORM → Tee
	    resonance[S]                 WORM → Tee
	    manifold[S]                  WORM → Tee
	             GROUP BARRIER

	stage 2:
	    cognition[S]                 WORM → Tee

	stage 3:
	    training[S]                  WORM → Tee

Each producer owns its own output Measurement. All handlers in one concurrent
HandlerGroup receive the SAME immutable StageInput from earlier completed stages.
A sibling MUST NOT become visible merely because it happened to finish first.
*/
type Workspace struct {
	*System
	channel  disruptor.Disruptor
	buffer   []*data.Measurement[float64]
	mask     int64
	capacity int

	// stages[stageIdx][nodeIdx] holds the Consumer for each node.
	// Each Consumer owns its preallocated output ring and published slots.
	stages [][]*Consumer

	// stageInputs[slot] holds the StageInput for each ring slot.
	// These are rebuilt at the start of each stage's Handle call to contain
	// the correct prior-stage outputs for that sequence.
	stageInputSlots [][]*StageInput // [stageIdx][slot]
}

func NewWorkspace(
	ctx context.Context,
	writerCount uint8,
	label string,
	stages [][]Node,
	tees ...Tee,
) *Workspace {
	capacity := int(system.Cfg.Runtime.Workspace.Buffer)
	mask := int64(capacity - 1)

	workspace := &Workspace{
		buffer:          make([]*data.Measurement[float64], capacity),
		mask:            mask,
		capacity:        capacity,
		stages:          make([][]*Consumer, len(stages)),
		stageInputSlots: make([][]*StageInput, len(stages)),
	}

	// Preallocate StageInput slots for each stage × ring slot.
	for stageIdx := range stages {
		workspace.stageInputSlots[stageIdx] = make([]*StageInput, capacity)
		for slot := 0; slot < capacity; slot++ {
			workspace.stageInputSlots[stageIdx][slot] = &StageInput{}
		}
	}

	opts := optionList(
		disruptor.Options.BufferCapacity(uint32(capacity)),
		disruptor.Options.WriterCount(writerCount),
	)

	for stageIdx, stageNodes := range stages {
		consumers := make([]*Consumer, 0, len(stageNodes))
		group := make([]disruptor.Handler, 0, len(stageNodes))

		for _, node := range stageNodes {
			consumer := NewConsumer(
				node,
				capacity,
				mask,
				workspace.stageInputSlots[stageIdx],
				tees...,
			)
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

// Step commits work without waiting for completion. The Disruptor's capacity
// barrier prevents reuse until all stages, including each consumer's Tee calls, finish.
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
		payload.SetSeqIdx(seq + 1)
	}

	workspace.buffer[seq&workspace.mask] = payload

	workspace.channel.Commit(seq, seq)

	return payload
}

/*
buildStageInput populates the StageInput for the given stage and sequence slot
with the ingress measurement and all completed prior-stage outputs.

This is called by stageHandler.Handle before delegating to Consumer.Handle so
that each handler in a concurrent group sees the SAME completed prior stages
but NEVER its own-stage siblings.
*/
func (workspace *Workspace) buildStageInput(stageIdx int, sequence int64) {
	slot := sequence & workspace.mask
	input := workspace.stageInputSlots[stageIdx][slot]

	input.seq = sequence + 1
	input.ingress = workspace.buffer[slot]

	// Collect outputs from all stages prior to stageIdx.
	if stageIdx > 0 {
		if cap(input.priorOutputs) >= stageIdx {
			input.priorOutputs = input.priorOutputs[:stageIdx]
		} else {
			input.priorOutputs = make([][]*data.Measurement[float64], stageIdx)
		}

		for priorStage := 0; priorStage < stageIdx; priorStage++ {
			priorConsumers := workspace.stages[priorStage]

			if cap(input.priorOutputs[priorStage]) >= len(priorConsumers) {
				input.priorOutputs[priorStage] = input.priorOutputs[priorStage][:len(priorConsumers)]
			} else {
				input.priorOutputs[priorStage] = make([]*data.Measurement[float64], len(priorConsumers))
			}

			for nodeIdx, consumer := range priorConsumers {
				input.priorOutputs[priorStage][nodeIdx] = consumer.Published(sequence)
			}
		}
	} else {
		input.priorOutputs = nil
	}
}

/*
stageHandler adapts Consumer to the disruptor.Handler interface while ensuring
the StageInput is populated with correct prior-stage outputs before Handle runs.
*/
type stageHandler struct {
	consumer  *Consumer
	workspace *Workspace
	stageIdx  int
}

func (sh *stageHandler) Handle(lower, upper int64) {
	for seq := lower; seq <= upper; seq++ {
		sh.workspace.buildStageInput(sh.stageIdx, seq)
	}

	sh.consumer.Handle(lower, upper)
}
