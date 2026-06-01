package user

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

type Execution struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	conn   *kraken.Client
}

func NewExecution(ctx context.Context) (*Execution, error) {
	ctx, cancel := context.WithCancel(ctx)

	execution := &Execution{
		ctx:    ctx,
		cancel: cancel,
		conn: errnie.Does(func() (*kraken.Client, error) {
			return kraken.NewClient(ctx)
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value(),
	}

	return execution, errnie.Error(errnie.Require(map[string]any{
		"ctx":    execution.ctx,
		"cancel": execution.cancel,
		"conn":   execution.conn,
	}))
}
