package trader

import (
	"context"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/symm/kraken/user"
)

type Execution struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	executions structure.Ring[user.Execution]
}

func NewExecution(ctx context.Context) *Execution {
	ctx, cancel := context.WithCancel(ctx)

	execution := &Execution{
		ctx:    ctx,
		cancel: cancel,
	}

	return execution
}

func (execution *Execution) Update(update user.Execution) {
	if execution.executions == nil {
		execution.executions = structure.NewListRing[user.Execution](
			1,
			datura.Acquire("execution", datura.Artifact_Type_json),
		)
	}
}

func (execution *Execution) Error() error {
	return execution.err
}

func (execution *Execution) Close() error {
	execution.cancel()
	return nil
}
