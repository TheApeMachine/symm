package market

import (
	"context"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

/*
Story holds the latest playbook verdicts per symbol for dashboards and audits.
*/
type Story struct {
	ctx            context.Context
	cancel         context.CancelFunc
	err            error
	pool           *qpool.Q[any]
	symbols        *sync.Map
	balances       *logic.Balances
	tree           *logic.Tree
	forwardPending *sync.Map
	forwardCal     *sync.Map
}

func NewStory(
	ctx context.Context,
	pool *qpool.Q[any],
) *Story {
	ctx, cancel := context.WithCancel(ctx)

	tree, err := logic.NewTree(ctx, pool)

	if err != nil {
		cancel()
		return nil
	}

	story := &Story{
		ctx:            ctx,
		cancel:         cancel,
		pool:           pool,
		symbols:        &sync.Map{},
		tree:           tree,
		forwardPending: &sync.Map{},
		forwardCal:     &sync.Map{},
	}

	return story
}

/*
Update processes an artifact carrying a collection of measurements
and returns a new artifact carrying the story's verdicts.
*/
func (story *Story) Update(artifact *datura.Artifact) *datura.Artifact {
	if story == nil || artifact == nil {
		return nil
	}

	measurements := datura.PeekPayload[[]*datura.Artifact](artifact, "measurements")

	if len(measurements) == 0 {
		return nil
	}

	verdicts, err := story.tree.Evaluate(
		measurements,
		story.balances,
		story.tree.Branches...,
	)

	if err != nil {
		return nil
	}

	return verdicts
}

/*
Error returns the story's error.
*/
func (story *Story) Error() error {
	return story.err
}

/*
Close shuts down the story.
*/
func (story *Story) Close() error {
	story.cancel()
	return nil
}
