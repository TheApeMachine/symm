package system

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

type StageType uint8

const (
	StagePreflight StageType = iota
	StageWarmup
	StageReady
)

type Stage struct {
	stageType StageType
	status    types.Status
	reporters []types.StatusReporter
}

func NewStage(stageType StageType, reporters ...types.StatusReporter) *Stage {
	return &Stage{
		stageType: stageType,
		status:    types.INITIALIZING,
		reporters: reporters,
	}
}

func (stage *Stage) Status() types.Status {
	for _, reporter := range stage.reporters {
		if reporter.Status() == types.ERROR {
			stage.status = types.ERROR
			return stage.status
		}

		if reporter.Status() != types.READY {
			if stage.status != types.INITIALIZING {
				stage.status = types.PENDING
				return stage.status
			}

			return stage.status
		}
	}

	stage.status = types.READY
	return stage.status
}

/*
Initialize brings up every reporter in registration order. Each reporter's
own Initialize runs, then Stage waits for that reporter to report READY
before moving to the next one, so a dependent reporter can rely on
ordering instead of polling its dependency's status itself.
*/
func (stage *Stage) Initialize() error {
	for _, reporter := range stage.reporters {
		if err := reporter.Initialize(); err != nil {
			stage.status = types.ERROR
			return errnie.Error(err)
		}

		for reporter.Status() != types.READY {
			if reporter.Status() == types.ERROR {
				stage.status = types.ERROR

				return errnie.Error(errnie.Err(
					errnie.Internal, "reporter failed to initialize", nil,
				))
			}

			time.Sleep(10 * time.Millisecond)
		}
	}

	return nil
}
