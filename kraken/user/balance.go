package user

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

type Balance struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	conn   *kraken.Client
}

func NewBalance(ctx context.Context) (*Balance, error) {
	ctx, cancel := context.WithCancel(ctx)

	conn := errnie.Does(func() (*kraken.Client, error) {
		return kraken.NewClient(ctx)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	balance := &Balance{
		ctx:    ctx,
		cancel: cancel,
		conn:   conn,
	}

	return balance, errnie.Error(errnie.Require(map[string]any{
		"ctx":    balance.ctx,
		"cancel": balance.cancel,
		"conn":   balance.conn,
	}))
}
