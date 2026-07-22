package system

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
Booter advances the registered initialization stages in dependency order.
*/
type Booter struct {
	ctx    context.Context
	cancel context.CancelFunc
	stages []*Stage
	uiHub  chan []byte
}

/*
NewBooter owns a cancellable boot sequence and its status publication channel.
*/
func NewBooter(ctx context.Context, uiHub chan []byte) *Booter {
	ctx, cancel := context.WithCancel(ctx)

	booter := &Booter{
		ctx:    ctx,
		cancel: cancel,
		stages: make([]*Stage, 0),
		uiHub:  uiHub,
	}

	return booter
}

/*
Start initializes every stage in order and stops at the first failure.
*/
func (booter *Booter) Start() error {
	for _, stage := range booter.stages {
		if err := stage.Initialize(booter.ctx, booter.uiHub); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}

/*
AddStages appends boot boundaries in their required dependency order.
*/
func (booter *Booter) AddStages(stages ...*Stage) {
	booter.stages = append(booter.stages, stages...)
}

/*
Ready reports whether one registered boundary has completed initialization.
*/
func (booter *Booter) Ready(stage StageType) bool {
	if booter == nil || int(stage) >= len(booter.stages) {
		return false
	}

	return booter.stages[stage].Status() == types.READY
}

/*
Error reports whether any registered stage reached an error state.
*/
func (booter *Booter) Error() bool {
	for _, stage := range booter.stages {
		if stage.Status() == types.ERROR {
			return true
		}
	}

	return false
}
