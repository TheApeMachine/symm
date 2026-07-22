package system

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
StageType identifies one ordered boot boundary.
*/
type StageType uint8

const (
	StagePreflight StageType = iota
	StageWarmup
	StageReady
)

/*
String returns the stable wire name published for a boot boundary.
*/
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

/*
Stage initializes one ordered group of readiness reporters.
*/
type Stage struct {
	stageType StageType
	status    atomic.Value
	reporters []types.StatusReporter
}

/*
NewStage retains the reporters that must become ready at one boot boundary.
*/
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

/*
Status derives the stage state from every registered reporter.
*/
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
func (stage *Stage) Initialize(
	ctx context.Context,
	uiHub chan<- []byte,
) error {
	readiness := time.NewTicker(10 * time.Millisecond)
	defer readiness.Stop()

	for _, reporter := range stage.reporters {
		if err := ctx.Err(); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Canceled,
				"stage initialization canceled",
				err,
			))
		}

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

		for {
			reporterStatus := reporter.Status()

			if reporterStatus == types.READY {
				break
			}

			if reporterStatus == types.ERROR {
				stage.status.Store(types.ERROR)
				stage.Publish(uiHub, datura.Map[any]{
					"stage":  stage.stageType.String(),
					"status": types.ERROR,
				})

				return errnie.Error(errnie.Err(
					errnie.Internal, "reporter failed to initialize", nil,
				))
			}

			select {
			case <-ctx.Done():
				return errnie.Error(errnie.Err(
					errnie.Canceled,
					"stage initialization canceled",
					ctx.Err(),
				))
			case <-readiness.C:
			}
		}
	}

	return nil
}

/*
Publish offers one boot-state frame without blocking initialization on the UI.
*/
func (stage *Stage) Publish(uiHub chan<- []byte, status datura.Map[any]) {
	select {
	case uiHub <- status.Marshal():
	default:
	}
}
