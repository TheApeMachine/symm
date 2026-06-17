package trader

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/trader/cognitive"
)

/*
Execution records fills and holds speculative pre-warm staging per symbol.
*/
type Execution struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	executions structure.Ring[user.Execution]
	staged     sync.Map
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

/*
PreWarm stores speculative execution staging keyed by symbol scope.
*/
func (execution *Execution) PreWarm(staging cognitive.Staging) {
	if execution == nil || staging.Scope == "" {
		return
	}

	if staging.PreparedAt <= 0 {
		staging.PreparedAt = time.Now().UnixNano()
	}

	execution.staged.Store(staging.Scope, staging)
}

/*
Staging returns the latest pre-warmed execution state for scope.
*/
func (execution *Execution) Staging(scope string) (cognitive.Staging, bool) {
	if execution == nil || scope == "" {
		return cognitive.Staging{}, false
	}

	raw, ok := execution.staged.Load(scope)

	if !ok {
		return cognitive.Staging{}, false
	}

	staging, ok := raw.(cognitive.Staging)

	return staging, ok
}

func (execution *Execution) Error() error {
	return execution.err
}

func (execution *Execution) Close() error {
	execution.cancel()
	return nil
}
