package system

import (
	"context"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

type Booter struct {
	ctx       context.Context
	cancel    context.CancelFunc
	stages    []*Stage
	uiHub     chan []byte
	lastPhase atomic.Value
}

func NewBooter(ctx context.Context, uiHub chan []byte) *Booter {
	ctx, cancel := context.WithCancel(ctx)

	booter := &Booter{
		ctx:    ctx,
		cancel: cancel,
		stages: make([]*Stage, 0),
		uiHub:  uiHub,
	}
	booter.lastPhase.Store("stream")

	go booter.publishLoop()

	return booter
}

/*
Phase maps one boot stage to the engine label shown in the terminal.
*/
func (stage StageType) Phase() string {
	switch stage {
	case StagePreflight:
		return "scan"
	case StageWarmup:
		return "evaluate"
	case StageReady:
		return "commit"
	default:
		return "stream"
	}
}

/*
CurrentPhase reports the active boot stage for the frontend engine panel.
*/
func (booter *Booter) CurrentPhase() string {
	if booter.Error() {
		return "error"
	}

	if len(booter.stages) == 0 {
		return "stream"
	}

	for stage := StagePreflight; stage <= StageReady; stage++ {
		if int(stage) >= len(booter.stages) {
			return stage.Phase()
		}

		if booter.stages[stage].Status() != types.READY {
			return stage.Phase()
		}
	}

	return StageReady.Phase()
}

func (booter *Booter) AddStages(stages ...*Stage) {
	booter.stages = append(booter.stages, stages...)

	for _, stage := range stages {
		errnie.Error(stage.Initialize())
	}

	booter.publishPhase(booter.CurrentPhase())
}

func (booter *Booter) Ready(stage StageType) bool {
	if int(stage) >= len(booter.stages) || booter.stages[stage] == nil {
		return false
	}

	return booter.stages[stage].Status() == types.READY
}

func (booter *Booter) Error() bool {
	for _, stage := range booter.stages {
		if stage.Status() == types.ERROR {
			return true
		}
	}

	return false
}

func (booter *Booter) publishLoop() {
	for booter.ctx.Err() == nil {
		phase := booter.CurrentPhase()
		lastPhase, _ := booter.lastPhase.Load().(string)

		if phase != lastPhase {
			booter.publishPhase(phase)
		}

		time.Sleep(50 * time.Millisecond)
		runtime.Gosched()
	}
}

func (booter *Booter) publishPhase(phase string) {
	booter.lastPhase.Store(phase)

	if booter.uiHub == nil {
		return
	}

	select {
	case booter.uiHub <- datura.Map[any]{
		"tick": datura.Map[any]{"phase": phase},
	}.Marshal():
	default:
		errnie.Error(errnie.Err(
			errnie.IO,
			"booter: UI channel full while publishing phase",
			nil,
		))
	}
}
