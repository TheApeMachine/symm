package market

import (
	"context"
	"errors"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/rawbus"
)

/*
Story holds the latest playbook verdicts per symbol for dashboards and audits.
*/
type Story struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	bus          *internal.Bus
	measurements *sync.Map
	tree         *logic.Tree
	crossSection *CrossSection
	regime       *RegimeClassifier
}

func NewStory(
	ctx context.Context, pool *qpool.Q[any],
) *Story {
	ctx, cancel := context.WithCancel(ctx)

	var err error
	tree, err := logic.NewTree()

	if errnie.Error(err) != nil {
		cancel()
		return nil
	}

	crossSection, err := LoadCrossSection(&CrossSectionOnce{})

	if errnie.Error(err) != nil {
		cancel()
		return nil
	}

	regime, err := NewRegimeClassifier(crossSection)

	if errnie.Error(err) != nil {
		cancel()
		return nil
	}

	return &Story{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		bus: internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelRaw, internal.ChannelUI, internal.ChannelAudit},
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelMeasurements, "story"),
			},
		),
		measurements: &sync.Map{},
		tree:         tree,
		crossSection: crossSection,
		regime:       regime,
		err:          err,
	}
}

/*
Tick joins measurements from perspective signals, evaluates the playbook, and
streams regime and decision-tree frames to the UI as measurements arrive.
*/
func (story *Story) Tick() error {
	if errnie.Error(story.err) != nil {
		return story.err
	}

	for {
		if story.ctx.Err() != nil {
			return story.ctx.Err()
		}

		row, err := story.bus.Poll("measurements")

		if internal.ReportError(err) != nil || row == nil {
			row, err = story.bus.Receive("measurements")
		}

		if internal.IsShutdown(err) {
			return err
		}

		if internal.ReportError(err) != nil {
			continue
		}

		if row == nil {
			continue
		}

		measurement, ok := row.Value.(logic.Measurement)

		if !ok {
			errnie.Error(errors.New("story: invalid measurement"))
			continue
		}

		if story.regime != nil {
			if err := story.regime.Observe(measurement); err != nil {
				return errnie.Error(err)
			}
		}

		if stats := story.tree.Stats(); stats != nil {
			stats.ObserveStoryTick()
		}

		raw, _ := story.measurements.LoadOrStore(measurement.Symbol, NewSymbolState())

		measurements := raw.(*SymbolState).Observe(measurement)

		var evaluation *logic.Evaluation

		if len(measurements) > 0 {
			evaluation = story.tree.Evaluate(measurements, nil)
		}

		if evaluation == nil || evaluation.Action == nil {
			story.publishUIFrames()
			continue
		}

		action := evaluation.Action

		if action.Symbol == "" {
			stamped := *action
			stamped.Symbol = measurement.Symbol
			action = &stamped
			evaluation.Action = action
		}

		errnie.Error(rawbus.Send(story.bus, rawbus.TypeActions, action))

		story.publishUIFrames()
	}
}

func (story *Story) publishUIFrames() {
	story.publishDecisionTree()
}

func (story *Story) publishDecisionTree() {
	stats := story.tree.Stats()

	if stats == nil {
		return
	}

	errnie.Error(story.bus.Send(internal.ChannelUI, "decision_tree", stats.DecisionTreeFrame()))
}

/*
Close shuts down the story.
*/
func (story *Story) Close() error {
	story.cancel()
	return story.bus.Close()
}
