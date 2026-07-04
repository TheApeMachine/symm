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
	symbols *sync.Map
	dirty   *sync.Map
	tree    *logic.Tree
}

func NewStory(
	ctx context.Context,
	pool *qpool.Q[any],
) *Story {
	ctx, cancel := context.WithCancel(ctx)

	tree, err := logic.NewTree(ctx, pool)

	story := &Story{
		ctx:     ctx,
		cancel:  cancel,
		symbols: &sync.Map{},
		dirty:   &sync.Map{},
		tree:    tree,
		err:     err,
	}

	return story
}

/*
Update evaluates playbook verdicts for the given scope measurements against the
supplied holdings, so playbook conditions (e.g. symbolHeld) see the live ledger.
*/
func (story *Story) Update(measurements []*datura.Artifact) {
	if story.symbols == nil {
		story.symbols = &sync.Map{}
	}

	if story.dirty == nil {
		story.dirty = &sync.Map{}
	}

	for _, measurement := range measurements {
		if measurement == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"story: nil measurement",
				nil,
			))
			continue
		}

		origin := datura.Peek[string](measurement, "origin")
		if origin == "" {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"story: measurement origin required",
				nil,
			).With(measurement.Log()...))
			continue
		}

		symbol := datura.Peek[string](measurement, "scope")
		if symbol == "" {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"story: measurement scope required",
				nil,
			).With(measurement.Log()...))
			continue
		}

		if datura.Peek[float64](measurement, "output", "value") <= 0 ||
			datura.Peek[float64](measurement, "output", "confidence") <= 0 ||
			datura.Peek[float64](measurement, "output", "entry_baseline") <= 0 ||
			datura.Peek[float64](measurement, "output", "exit_baseline") <= 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"story: measurement output contract required",
				nil,
			).With(measurement.Log()...))
			continue
		}

		ring, _ := story.symbols.LoadOrStore(
			symbol, structure.NewListRing[*datura.Artifact](64),
		)

		ring.(*structure.ListRing[*datura.Artifact]).Push(
			measurement,
		)
		story.dirty.Store(symbol, true)
	}
}

/*
Actions lazily evaluates the decision tree, and potentially generates
candidate actions, which are used by the trader as a mechanism to scope
down the measurements into something it can reason about and make choices.
*/
func (story *Story) Actions(balances *datura.Artifact) []*datura.Artifact {
	actions, _ := story.ActionsWithTrace(balances)
	return actions
}

func (story *Story) ActionsWithTrace(balances *datura.Artifact) ([]*datura.Artifact, []*datura.Artifact) {
	actions := make([]*datura.Artifact, 0)
	traces := make([]*datura.Artifact, 0)

	if story.dirty == nil {
		return actions, traces
	}

	if story.symbols == nil {
		return actions, traces
	}

	story.dirty.Range(func(key any, _ any) bool {
		story.dirty.Delete(key)

		value, ok := story.symbols.Load(key)
		if !ok {
			return true
		}

		ring, _ := value.(*structure.ListRing[*datura.Artifact])
		latest := make(map[string]*datura.Artifact)

		ring.Do(func(measurement *datura.Artifact) {
			if measurement == nil {
				return
			}

			origin := datura.Peek[string](measurement, "origin")
			if origin == "" {
				return
			}

			if current := latest[origin]; current == nil || measurement.Timestamp() >= current.Timestamp() {
				latest[origin] = measurement
			}
		})

		measurements := make([]*datura.Artifact, 0, len(latest))
		for _, measurement := range latest {
			measurements = append(measurements, measurement)
		}

		candidates, err := story.tree.Evaluate(measurements, balances, story.tree.Branches)

		if err != nil {
			errnie.Error(err)
			return true
		}

		for _, measurement := range measurements {
			measurement.WithAttribute("journey.story.evaluated", true)
			measurement.WithAttribute("journey.story.candidates", len(candidates))

			if datura.Peek[string](measurement, "journey", "story", "terminal") != "" {
				traces = append(traces, measurement)
			}
		}

		for _, candidate := range candidates {
			payload, err := sonic.Marshal(candidate)

			if err != nil {
				errnie.Error(err)
			}

			action := datura.Acquire(
				"story", datura.APPJSON,
			).WithPayload(
				payload,
			).WithRole(
				string(candidate.Side),
			).WithScope(
				candidate.Symbol,
			).WithAttribute(
				"journey.story.status", "candidate",
			).WithAttribute(
				"journey.story.symbol", candidate.Symbol,
			).WithAttribute(
				"journey.story.source", string(candidate.ReasonSource),
			).WithAttribute(
				"journey.story.category", string(candidate.ReasonCategory),
			)

			actions = append(actions, action)
		}

		return true
	})

	return actions, traces
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
