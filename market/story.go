package market

import (
	"context"
	"errors"
	"sync"

	"github.com/spf13/viper"
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
	ctx                 context.Context
	cancel              context.CancelFunc
	err                 error
	pool                *qpool.Q[any]
	bus                 *internal.Bus
	symbols             *sync.Map
	holdings            *logic.Holdings
	tree                *logic.Tree
	crossSection        *CrossSection
	regime              *RegimeClassifier
	bufferSize          int
	storyTicks          int
	playbookEvaluations int
}

func NewStory(
	ctx context.Context, pool *qpool.Q[any],
) *Story {
	ctx, cancel := context.WithCancel(ctx)

	bus := internal.NewBus(
		ctx,
		pool,
		[]internal.Channel{internal.ChannelRaw, internal.ChannelUI, internal.ChannelAudit},
		[]internal.Subscription{
			internal.Subscribe(internal.ChannelMeasurements, "story"),
		},
	)

	var err error
	tree, err := logic.NewTree(bus)

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

	bufferSize := viper.GetInt("story.measurements.buffer")

	if bufferSize <= 0 {
		cancel()
		return nil
	}

	story := &Story{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		bus:          bus,
		symbols:      &sync.Map{},
		holdings:     logic.NewHoldings(),
		tree:         tree,
		crossSection: crossSection,
		regime:       regime,
		bufferSize:   bufferSize,
	}

	story.publishStatusUI()

	return story
}

/*
Tick joins measurements from perspective signals, evaluates the playbook, and
streams the embedded tree and regime frames to the UI.
*/
func (story *Story) Tick() (err error) {
	if errnie.Error(story.err) != nil {
		return story.err
	}

	var (
		row         *qpool.QValue[any]
		measurement logic.Measurement
		ok          bool
	)

	for {
		if story.ctx.Err() != nil {
			return story.ctx.Err()
		}

		if row, err = story.bus.Receive("measurements"); err != nil {
			if internal.IsShutdown(err) {
				return err
			}

			return errnie.Error(err)
		}

		switch rawbus.TypeFrom(row.Type) {
		case rawbus.TypeOrder:
			action, err := rawbus.DecodeAction(row)

			if err != nil {
				return errnie.Error(err)
			}

			if action == nil {
				return errnie.Error(errors.New("story: invalid action"))
			}

			story.holdings.SetPosition(
				action.Symbol,
				action.Quantity,
			)
		case rawbus.TypeMeasurements:
			if measurement, ok = row.Value.(logic.Measurement); !ok {
				return errnie.Error(errors.New("story: invalid measurement"))
			}

			if story.regime == nil {
				return errnie.Error(errors.New("story: regime is nil"))
			}

			if err := story.regime.Observe(measurement); err != nil {
				return errnie.Error(err)
			}

			if err := story.ingestMeasurement(measurement); err != nil {
				return errnie.Error(err)
			}
		}
	}
}

func (story *Story) ingestMeasurement(measurement logic.Measurement) error {
	raw, _ := story.symbols.LoadOrStore(
		measurement.Symbol,
		newSymbolState(story.bufferSize),
	)

	state := raw.(*symbolState)

	complete, err := state.absorb(measurement)

	if err != nil {
		return err
	}

	if !complete {
		return nil
	}

	if story.regime != nil {
		if err := story.regime.PublishFrame(story.bus); err != nil {
			return errnie.Error(err)
		}
	}

	state.appendSpectrum()
	state.resetSpectrum()

	story.storyTicks++

	ordered := state.orderedMeasurements()

	walkTrace := &logic.WalkTrace{Symbol: measurement.Symbol}

	evaluation, nextWalk, err := story.tree.EvaluateContinuing(
		ordered,
		story.holdings,
		state.walk,
		walkTrace,
	)

	if errnie.Error(err) != nil {
		return errnie.Err(
			errnie.Validation,
			"story: failed to evaluate playbook",
			err,
		)
	}

	story.playbookEvaluations++
	state.walk = nextWalk

	if err := story.publishStatusUI(); err != nil {
		return err
	}

	if err := story.publishWalkTrace(walkTrace); err != nil {
		return err
	}

	if evaluation != nil && evaluation.Action != nil {
		action := evaluation.Action

		if action.Symbol == "" {
			stamped := *action
			stamped.Symbol = measurement.Symbol
			action = &stamped
			evaluation.Action = action
		}

		state.walk = nil

		return rawbus.Send(story.bus, rawbus.TypeActions, action)
	}

	return nil
}

func (story *Story) publishStatusUI() error {
	return story.bus.Send(internal.ChannelUI, "story", map[string]any{
		"story_ticks":          story.storyTicks,
		"playbook_evaluations": story.playbookEvaluations,
	})
}

func (story *Story) publishWalkTrace(trace *logic.WalkTrace) error {
	if trace == nil || len(trace.Steps) == 0 {
		return nil
	}

	return story.bus.Send(internal.ChannelUI, "decision_walk", trace)
}

/*
Close shuts down the story.
*/
func (story *Story) Close() error {
	story.cancel()
	return story.bus.Close()
}
