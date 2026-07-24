package system

import (
	"context"
	"sync/atomic"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/types"
)

type StageType uint8

const (
	StagePreflight StageType = iota
	StageWarmup
	StageReady
)

func (stageType StageType) String() string {
	switch stageType {
	case StagePreflight:
		return "preflight"
	case StageWarmup:
		return "warmup"
	case StageReady:
		return "ready"
	}

	return "unknown"
}

type Stage struct {
	stageType StageType
	status    atomic.Value
	reporters []types.StatusReporter
}

func NewStage(
	stageType StageType,
	reporters ...types.StatusReporter,
) *Stage {
	stage := &Stage{
		stageType: stageType,
		reporters: reporters,
	}

	stage.status.Store(types.INITIALIZING)

	return stage
}

func (stage *Stage) Status() types.Status {
	status := stage.status.Load()

	if status == nil {
		status = types.INITIALIZING
		stage.status.Store(status)
	}

	for _, reporter := range stage.reporters {
		reporterStatus := reporter.Status()

		if reporterStatus == types.ERROR {
			stage.status.Store(types.ERROR)
			return types.ERROR
		}

		if reporterStatus != types.READY {
			if status.(types.Status) != types.INITIALIZING {
				stage.status.Store(types.PENDING)
				return types.PENDING
			}

			return status.(types.Status)
		}
	}

	stage.status.Store(types.READY)
	return types.READY
}

/*
Initialize brings up every reporter in registration order. Each reporter's own
Initialize runs, then Stage waits on a context-aware readiness future before
moving to the next one so boot cannot hang without a deadline forever.
*/
func (stage *Stage) Initialize(ctx context.Context, uiHub chan<- []byte) error {
	if ctx == nil {
		ctx = context.Background()
	}

	for _, reporter := range stage.reporters {
		stage.Publish(uiHub, datura.Map[any]{
			"stage":  stage.stageType.String(),
			"status": stage.Status(),
		})

		if err := reporter.Initialize(); err != nil {
			stage.status.Store(types.ERROR)
			stage.Publish(uiHub, datura.Map[any]{
				"stage":  stage.stageType.String(),
				"status": types.ERROR,
			})

			return err
		}

		if err := waitReporter(ctx, reporter); err != nil {
			stage.status.Store(types.ERROR)
			stage.Publish(uiHub, datura.Map[any]{
				"stage":  stage.stageType.String(),
				"status": types.ERROR,
			})

			return err
		}
	}

	return nil
}

/*
waitReporter prefers an explicit ReadyFuture when the reporter exposes one.
*/
func waitReporter(ctx context.Context, reporter types.StatusReporter) error {
	if future, ok := reporter.(interface{ Ready() *types.ReadyFuture }); ok {
		return future.Ready().Wait(ctx)
	}

	return types.WaitStatus(ctx, reporter)
}

func (stage *Stage) Publish(uiHub chan<- []byte, status datura.Map[any]) {
	if uiHub == nil {
		return
	}

	frame, err := status.Marshal()

	if err != nil {
		return
	}

	select {
	case uiHub <- frame:
	default:
	}
}
