package market

import (
	"context"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

/*
Story holds the latest playbook verdicts per symbol for dashboards and audits.
*/
type Story struct {
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	pool     *qpool.Q[any]
	symbols  *sync.Map
	balances *logic.Balances
	tree     *logic.Tree
}

func NewStory(
	ctx context.Context,
	pool *qpool.Q[any],
) *Story {
	ctx, cancel := context.WithCancel(ctx)

	tree, err := logic.NewTree(ctx, pool)

	if err != nil {
		cancel()
		errnie.Error(errnie.Err(
			errnie.Validation,
			"story: failed to create tree",
			err,
		))
	}

	story := &Story{
		ctx:     ctx,
		cancel:  cancel,
		pool:    pool,
		symbols: &sync.Map{},
		tree:    tree,
	}

	return story
}

/*
Update evaluates playbook verdicts for the given scope measurements.
*/
func (story *Story) Update(
	measurements []*datura.Artifact,
) []*datura.Artifact {
	if story == nil || len(measurements) == 0 {
		return nil
	}

	actions, err := story.tree.Evaluate(
		measurements,
		story.balances,
		story.tree.Branches,
	)

	if err != nil {
		story.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"story: playbook evaluation failed",
			err,
		))

		return nil
	}

	artifacts := make([]*datura.Artifact, 0)

	for _, action := range actions {
		buf, err := sonic.Marshal(action)

		if err != nil {
			story.err = errnie.Error(errnie.Err(
				errnie.Validation,
				"story: failed to marshal action",
				err,
			))

			return nil
		}

		artifact := datura.Acquire("story", datura.APPJSON)
		artifact.WithRole(string(action.Side))
		artifact.WithScope(action.Symbol)
		artifact.WithPayload(buf)
		artifacts = append(artifacts, artifact)
	}

	return artifacts
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
