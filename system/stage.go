package system

import (
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
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
Initialize brings up every reporter in registration order. Each reporter's
own Initialize runs, then Stage waits for that reporter to report READY
before moving to the next one, so a dependent reporter can rely on
ordering instead of polling its dependency's status itself.
*/
func (stage *Stage) Initialize(uiHub chan<- []byte) error {
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

			return errnie.Error(err)
		}

		for reporter.Status() != types.READY {
			if reporter.Status() == types.ERROR {
				stage.status.Store(types.ERROR)
				stage.Publish(uiHub, datura.Map[any]{
					"stage":  stage.stageType.String(),
					"status": types.ERROR,
				})

				return errnie.Error(errnie.Err(
					errnie.Internal, "reporter failed to initialize", nil,
				))
			}

			time.Sleep(10 * time.Millisecond)
		}
	}

	return nil
}

func (stage *Stage) Publish(uiHub chan<- []byte, status datura.Map[any]) {
	select {
	case uiHub <- status.Marshal():
	default:
	}
}
