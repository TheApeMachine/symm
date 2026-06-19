package broker

import (
	"context"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

/*
Desk routes playbook actions to the Kraken private artifact bus.
*/
type Desk struct {
	ctx    context.Context
	cancel context.CancelFunc
	pool   *qpool.Q[any]
	tree   *dmt.Tree
}

func NewDesk(
	ctx context.Context, pool *qpool.Q[any], tree *dmt.Tree,
) *Desk {
	ctx, cancel := context.WithCancel(ctx)

	return &Desk{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		tree:   tree,
	}
}

func (desk *Desk) Close() error {
	desk.cancel()

	return nil
}
