package system

import "github.com/theapemachine/symm/types"

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
			return types.ERROR
		}

		if reporter.Status() != types.READY {
			if stage.status != types.INITIALIZING {
				return types.PENDING
			}

			return types.INITIALIZING
		}
	}

	return types.READY
}
