package market

import (
	"context"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
)

const storyCapacity = 64

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

type storySymbol struct {
	measurements []*logic.Measurement
}

func NewStory(ctx context.Context) *Story {
	ctx, cancel := context.WithCancel(ctx)
	tree, err := logic.NewTree(ctx)

	return &Story{
		ctx:     ctx,
		cancel:  cancel,
		symbols: &sync.Map{},
		dirty:   &sync.Map{},
		tree:    tree,
		err:     err,
	}
}

/*
Update evaluates playbook verdicts for the given scope measurements against the
supplied holdings, so playbook conditions see the live ledger.
*/
func (story *Story) Update(measurements []*logic.Measurement) error {
	if story.symbols == nil {
		story.symbols = &sync.Map{}
	}

	if story.dirty == nil {
		story.dirty = &sync.Map{}
	}

	for _, measurement := range measurements {
		if err := measurement.Ready(); err != nil {
			return errnie.Error(err)
		}

		value, _ := story.symbols.LoadOrStore(
			measurement.Symbol,
			&storySymbol{measurements: make([]*logic.Measurement, 0, storyCapacity)},
		)

		value.(*storySymbol).Push(measurement)
		story.dirty.Store(measurement.Symbol, true)
	}

	return nil
}

/*
Actions lazily evaluates the decision tree and returns candidate actions.
*/
func (story *Story) Actions(
	holdings *logic.Holdings,
) ([]*logic.Action, error) {
	actions, _, err := story.ActionsWithTrace(holdings)
	return actions, err
}

func (story *Story) ActionsWithTrace(
	holdings *logic.Holdings,
) ([]*logic.Action, []*logic.Measurement, error) {
	actions := make([]*logic.Action, 0)
	traces := make([]*logic.Measurement, 0)

	if story.err != nil {
		return nil, nil, story.err
	}

	if story.dirty == nil || story.symbols == nil {
		return actions, traces, nil
	}

	var err error
	story.dirty.Range(func(key any, _ any) bool {
		story.dirty.Delete(key)

		value, ok := story.symbols.Load(key)
		if !ok {
			return true
		}

		symbol, ok := value.(*storySymbol)
		if !ok {
			err = errnie.Err(errnie.Validation, "story: invalid symbol measurements", nil)
			return false
		}

		measurements := symbol.Latest()
		candidates, evaluateErr := story.tree.Evaluate(
			measurements,
			holdings,
			story.tree.Branches,
		)
		if evaluateErr != nil {
			err = evaluateErr
			return false
		}

		for _, measurement := range measurements {
			measurement.Story.Evaluated = true
			measurement.Story.Candidates = len(candidates)

			if measurement.Story.Terminal != "" {
				traces = append(traces, measurement)
			}
		}

		actions = append(actions, candidates...)
		return true
	})

	if err != nil {
		return nil, nil, errnie.Error(err)
	}

	return actions, traces, nil
}

func (symbol *storySymbol) Push(measurement *logic.Measurement) {
	symbol.measurements = append(symbol.measurements, measurement)
	if len(symbol.measurements) <= storyCapacity {
		return
	}

	copy(symbol.measurements, symbol.measurements[len(symbol.measurements)-storyCapacity:])
	symbol.measurements = symbol.measurements[:storyCapacity]
}

func (symbol *storySymbol) Latest() []*logic.Measurement {
	latest := make(map[logic.SourceType]*logic.Measurement)

	for _, measurement := range symbol.measurements {
		current := latest[measurement.Source]
		if current == nil || measurement.At.After(current.At) || measurement.At.Equal(current.At) {
			latest[measurement.Source] = measurement
		}
	}

	measurements := make([]*logic.Measurement, 0, len(latest))
	for _, measurement := range latest {
		measurements = append(measurements, measurement)
	}

	return measurements
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
