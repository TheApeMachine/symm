package broker

import (
	"context"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

type StopLoss struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	pool   *qpool.Q[any]
	tree   *dmt.Tree
}

func NewStopLoss(
	ctx context.Context, pool *qpool.Q[any], tree *dmt.Tree,
) *StopLoss {
	ctx, cancel := context.WithCancel(ctx)

	return &StopLoss{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		tree:   tree,
	}
}

func (sl *StopLoss) Update(artifact *datura.Artifact) error {
	return nil
}

func (sl *StopLoss) Close() error {
	sl.cancel()
	return nil
}

func (sl *StopLoss) Error() error {
	return sl.err
}
