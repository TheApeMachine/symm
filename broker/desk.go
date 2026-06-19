package broker

import (
	"context"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

/*
Desk is the link between the trader and the Kraken exchange. It is responsible
for managing orders and executions, as well as keeping track of stoplosses,
managing risk, etc.
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

/*
Update the desk about any new price movements, balance changes, or other events
that are relevant for the desk to do its job.
*/
func (desk *Desk) Update(artifact *datura.Artifact) error {
	return nil
}

func (desk *Desk) Close() error {
	desk.cancel()

	return nil
}
