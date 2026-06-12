package market

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/config"
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
	tradingConfig       config.TradingConfig
	bufferSize          int
	storyTicks          int
	playbookEvaluations int
	recorder            *audit.Recorder
}

func NewStory(
	ctx context.Context, pool *qpool.Q[any],
) (*Story, error) {
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
		return nil, err
	}

	crossSection, err := LoadCrossSection(&CrossSectionOnce{})

	if errnie.Error(err) != nil {
		cancel()
		return nil, err
	}

	regime, err := NewRegimeClassifier(crossSection)

	if errnie.Error(err) != nil {
		cancel()
		return nil, err
	}

	bufferSize := viper.GetInt("story.measurements.buffer")

	if bufferSize <= 0 {
		cancel()
		return nil, errnie.Error(fmt.Errorf(
			"story: story.measurements.buffer must be positive, got %d",
			bufferSize,
		))
	}

	recorder, err := openAuditRecorder()

	if errnie.Error(err) != nil {
		cancel()
		return nil, err
	}

	tradingConfig, err := config.LoadTradingConfig()

	if errnie.Error(err) != nil {
		cancel()
		return nil, err
	}

	story := &Story{
		ctx:           ctx,
		cancel:        cancel,
		pool:          pool,
		bus:           bus,
		symbols:       &sync.Map{},
		holdings:      logic.NewHoldings(),
		tree:          tree,
		crossSection:  crossSection,
		regime:        regime,
		tradingConfig: tradingConfig,
		bufferSize:    bufferSize,
		recorder:      recorder,
	}

	return story, nil
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
				action.EntryConfidence,
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

			if story.regime != nil {
				if err := story.regime.PublishFrame(story.bus); err != nil {
					return errnie.Error(err)
				}
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

	if story.regime != nil {
		if err := story.regime.PublishFrame(story.bus); err != nil {
			return errnie.Error(err)
		}
	}

	if complete {
		state.appendSpectrum()
		state.resetSpectrum()
		story.storyTicks++
	}

	ordered := state.orderedMeasurements()
	gaugeReadings := state.slotMeasurements()

	walkTrace := &logic.WalkTrace{Symbol: measurement.Symbol}

	thresholdCtx := story.thresholdContext()

	evaluation, nextWalk, err := story.tree.EvaluateContinuing(
		ordered,
		story.holdings,
		state.walk,
		walkTrace,
		&thresholdCtx,
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

	story.bus.Send(internal.ChannelUI, "state", map[string]any{
		"measurements":         ordered,
		"gauge_readings":       gaugeReadings,
		"walk":                 state.walk,
		"story_ticks":          story.storyTicks,
		"playbook_evaluations": story.playbookEvaluations,
		"decision_walk":        walkTrace,
	})

	if evaluation != nil && evaluation.Action != nil {
		action := evaluation.Action

		if action.Symbol == "" {
			stamped := *action
			stamped.Symbol = measurement.Symbol
			action = &stamped
			evaluation.Action = action
		}

		prepared, prepareErr := prepareAction(
			story.holdings,
			action,
			ordered,
			story.tradingConfig,
		)

		if prepareErr != nil {
			return errnie.Error(prepareErr)
		}

		if prepared == nil {
			if err := story.writePlaybookTrace(measurement.Symbol, walkTrace); err != nil {
				return errnie.Error(err)
			}

			return nil
		}

		action = prepared
		evaluation.Action = action
		state.walk = nil

		if err := story.writePlaybookAction(measurement.Symbol, action); err != nil {
			return errnie.Error(err)
		}

		return rawbus.Send(story.bus, rawbus.TypeActions, action)
	}

	if err := story.writePlaybookTrace(measurement.Symbol, walkTrace); err != nil {
		return errnie.Error(err)
	}

	return nil
}

/*
Close shuts down the story.
*/
func (story *Story) Close() error {
	story.cancel()

	if story.recorder != nil {
		if err := story.recorder.Close(); err != nil {
			return err
		}
	}

	return story.bus.Close()
}

func openAuditRecorder() (*audit.Recorder, error) {
	if !viper.GetBool("system.audit.enabled") {
		return nil, nil
	}

	return audit.NewRecorder(viper.GetString("system.audit.file"))
}

func (story *Story) writePlaybookAction(symbol string, action *logic.Action) error {
	if story.recorder == nil {
		return nil
	}

	return story.recorder.Write(map[string]any{
		"symbol": symbol,
		"action": action,
	})
}

func (story *Story) writePlaybookTrace(symbol string, walkTrace *logic.WalkTrace) error {
	if story.recorder == nil {
		return nil
	}

	return story.recorder.Write(map[string]any{
		"symbol": symbol,
		"trace":  walkTrace.EvaluationSummary(),
	})
}
