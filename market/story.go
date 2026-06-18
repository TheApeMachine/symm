package market

import (
	"context"
	"strings"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
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

func (story *Story) Measurements() []logic.Measurement {
	measurements := make([]logic.Measurement, 0, logic.SourceCount)

	story.symbols.Range(func(_, value any) bool {
		sources := value.(*sync.Map)

		sources.Range(func(_, measurement any) bool {
			measurements = append(measurements, measurement.(logic.Measurement))

			return true
		})

		return true
	})

	return measurements
}

func (story *Story) DecisionTreeBranches() []*logic.Branch {
	if story.tree == nil {
		return nil
	}

	return story.tree.Branches
}

func (story *Story) Actions() []*logic.Action {
	if story.balances == nil {
		return nil
	}

	actions := make([]*logic.Action, 0)

	story.symbols.Range(func(key, value any) bool {
		sources := value.(*sync.Map)
		measurements := make([]logic.Measurement, 0, logic.SourceCount)

		sources.Range(func(_, measurement any) bool {
			measurements = append(measurements, measurement.(logic.Measurement))

			return true
		})

		if len(measurements) == 0 {
			return true
		}

		results, _ := story.tree.Evaluate(
			measurements, story.balances, story.tree.Branches,
		)

		for _, action := range results {
			if action != nil {
				actions = append(actions, action)
			}
		}

		return true
	})

	return actions
}

/*
PlaybookEvaluationCount reports how many symbol playbooks were evaluated this tick.
*/
func (story *Story) PlaybookEvaluationCount() int {
	count := 0

	story.symbols.Range(func(_, value any) bool {
		sources := value.(*sync.Map)
		hasMeasurement := false

		sources.Range(func(_, _ any) bool {
			hasMeasurement = true

			return false
		})

		if hasMeasurement {
			count++
		}

		return true
	})

	return count
}

/*
AnchorWalkTrace returns the playbook walk for the configured anchor symbol.
*/
func (story *Story) AnchorWalkTrace() logic.WalkTrace {
	if story.tree == nil || story.balances == nil {
		return logic.WalkTrace{}
	}

	anchor := strings.TrimSpace(viper.GetString("market.anchor_symbol"))

	if anchor == "" {
		return logic.WalkTrace{}
	}

	return story.WalkTrace(anchor)
}

/*
WalkTrace evaluates the embedded playbook for one symbol and records descent steps.
*/
func (story *Story) WalkTrace(symbol string) logic.WalkTrace {
	if story.tree == nil || story.balances == nil || symbol == "" {
		return logic.WalkTrace{}
	}

	raw, ok := story.symbols.Load(symbol)

	if !ok {
		return logic.WalkTrace{Symbol: symbol}
	}

	sources := raw.(*sync.Map)
	measurements := make([]logic.Measurement, 0, logic.SourceCount)

	sources.Range(func(_, measurement any) bool {
		measurements = append(measurements, measurement.(logic.Measurement))

		return true
	})

	return logic.WalkTree(symbol, measurements, story.balances, story.tree.Branches)
}

func (story *Story) Update(artifact *datura.Artifact) error {
	switch datura.Peek[string](artifact, "role") {
	case "measurement":
		measurement, ok := logic.MeasurementFromArtifact("", artifact)

		if !ok {
			return nil
		}

		if measurement.Symbol == "" {
			measurement.Symbol = datura.Peek[string](artifact, "scope")
		}

		if measurement.Symbol == "" || measurement.Source == "" {
			return nil
		}

		sources, _ := story.symbols.LoadOrStore(measurement.Symbol, &sync.Map{})
		sources.(*sync.Map).Store(measurement.Source, measurement)
	case "balances":
		payload := datura.As[logic.Balances](artifact)
		story.balances = &payload
	}

	return nil
}

/*
Close shuts down the story.
*/
func (story *Story) Close() error {
	story.cancel()
	return nil
}
