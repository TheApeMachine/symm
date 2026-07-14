package system

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

type Booter struct {
	ctx    context.Context
	cancel context.CancelFunc
	stages []*Stage
	uiHub  chan []byte
}

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

func (booter *Booter) Start() error {
	for _, stage := range booter.stages {
		if err := stage.Initialize(booter.uiHub); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}

func (booter *Booter) AddStages(stages ...*Stage) {
	booter.stages = append(booter.stages, stages...)
}

func (booter *Booter) Ready(stage StageType) bool {
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
