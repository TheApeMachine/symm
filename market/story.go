package market

import (
	"context"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
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
) *Story {
	ctx, cancel := context.WithCancel(ctx)

	story := &Story{
		ctx:     ctx,
		cancel:  cancel,
		pool:    pool,
		symbols: &sync.Map{},
		tree: errnie.Does(func() (*logic.Tree, error) {
			return logic.NewTree(ctx, pool)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.Validation,
				err.Error(),
				err,
			))
		}).Value(),
	}

	return story
}

/*
Update evaluates playbook verdicts for the given scope measurements against the
supplied holdings, so playbook conditions (e.g. symbolHeld) see the live ledger.
*/
func (story *Story) Update(measurements []*datura.Artifact) {
	for _, measurement := range measurements {
		symbol := datura.Peek[string](measurement, "scope")

		ring, _ := story.symbols.LoadOrStore(
			symbol, structure.NewListRing[*datura.Artifact](64),
		)

		ring.(*structure.ListRing[*datura.Artifact]).Push(
			measurement,
		)
	}
}

/*
Actions lazily evaluates the decision tree, and potentially generates
candidate actions, which are used by the trader as a mechanism to scope
down the measurements into something it can reason about and make choices.
*/
func (story *Story) Actions(balances *datura.Artifact) []*datura.Artifact {
	actions := make([]*datura.Artifact, 0)

	story.symbols.Range(func(key any, value any) bool {
		ring, _ := value.(*structure.ListRing[*datura.Artifact])
		measurements := make([]*datura.Artifact, 0)

		ring.Do(func(measurement *datura.Artifact) {
			if measurement == nil {
				return
			}

			measurements = append(measurements, measurement)
		})

		candidates, err := story.tree.Evaluate(
			measurements, balances, story.tree.Branches,
		)

		if err != nil {
			errnie.Error(err)
		}

		for _, candidate := range candidates {
			payload, err := sonic.Marshal(candidate)

			if err != nil {
				errnie.Error(err)
			}

			actions = append(actions, datura.Acquire(
				"story", datura.APPJSON,
			).WithPayload(
				payload,
			).WithRole(
				string(candidate.Side),
			).WithScope(
				candidate.Symbol,
			))
		}

		return true
	})

	return actions
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
