package market

import (
	"context"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
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

	tree, err := logic.NewTree(ctx, pool)

	if err != nil {
		cancel()
		return nil
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

func (story *Story) Actions() []*logic.Action {
	story.symbols.Range(func(key, value any) bool {
		measurements := value.(*structure.ClockRing[[]logic.Measurement])
		measurements.Do(func(slot structure.ClockSlot[[]logic.Measurement]) {
			story.tree.Evaluate(slot.Payload, nil, nil)
		})

		return true
	})

	return nil
}

func (story *Story) Update(artifact *datura.Artifact) error {
	scope := artifact.Peek("scope")
	measurement := datura.As[logic.Measurement](artifact)

	symbol, _ := story.symbols.LoadOrStore(
		scope, structure.NewClockRing[logic.Measurement](
			10, 100, 1000,
			datura.Acquire(
				"story", datura.Artifact_Type_json,
			).WithRole(
				"measurement",
			).WithScope(
				scope,
			),
		),
	)

	ring := symbol.(*structure.ClockRing[logic.Measurement])

	ring.ObserveSecond(
		measurement.ObservedAt, measurement,
	)

	return nil
}

/*
Close shuts down the story.
*/
func (story *Story) Close() error {
	story.cancel()
	return nil
}
