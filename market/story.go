package market

import (
	"context"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

/*
Story holds the latest playbook verdicts per symbol for dashboards and audits.
*/
type Story struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	pool    *qpool.Q[any]
	symbols *sync.Map
	tree    *logic.Tree
}

func NewStory(
	ctx context.Context,
	pool *qpool.Q[any],
) (*Story, error) {
	ctx, cancel := context.WithCancel(ctx)

	tree, err := logic.NewTree(ctx, pool)

	if err != nil {
		cancel()
		return nil, errnie.Error(err)
	}

	story := &Story{
		ctx:     ctx,
		cancel:  cancel,
		pool:    pool,
		symbols: &sync.Map{},
		tree:    tree,
	}

	return story, nil
}

/*
Close shuts down the story.
*/
func (story *Story) Close() error {
	story.cancel()
	return nil
}
