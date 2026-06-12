package market

import (
	"context"
	"errors"
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
	paperWalletQuote    float64
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
		[]internal.Channel{
			internal.ChannelRaw,
			internal.ChannelUI,
			internal.ChannelAudit,
		},
		[]internal.Subscription{
			internal.Subscribe(
				internal.ChannelMeasurements,
				"story",
			),
		},
	)

	tree, err := logic.NewTree(bus)

	if err != nil {
		cancel()
		return nil, errnie.Error(err)
	}

	crossSection, err := LoadCrossSection(&CrossSectionOnce{})

	if err != nil {
		cancel()
		return nil, errnie.Error(err)
	}

	regime, err := NewRegimeClassifier(crossSection)

	if err != nil {
		cancel()
		return nil, errnie.Error(err)
	}

	bufferSize := viper.GetInt("story.measurements.buffer")

	if bufferSize <= 0 {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"story: story.measurements.buffer must be positive, got %d",
			errors.New("buffer size is not positive"),
		))
	}

	tradingConfig, err := config.LoadTradingConfig()

	if err != nil {
		cancel()
		return nil, errnie.Error(err)
	}

	paperWalletQuote := 0.0

	if tradingConfig.Model == "paper" {
		paperWalletQuote, err = config.PaperWalletBalance()

		if err != nil {
			cancel()
			return nil, errnie.Error(err)
		}
	}

	story := &Story{
		ctx:              ctx,
		cancel:           cancel,
		pool:             pool,
		bus:              bus,
		symbols:          &sync.Map{},
		holdings:         logic.NewHoldings(),
		tree:             tree,
		crossSection:     crossSection,
		regime:           regime,
		tradingConfig:    tradingConfig,
		paperWalletQuote: paperWalletQuote,
		bufferSize:       bufferSize,
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

var evaluationPool = sync.Pool{
	New: func() any {
		return &logic.Evaluation{}
	},
}

func (story *Story) ingestMeasurement(measurement logic.Measurement) (err error) {
	raw, _ := story.symbols.LoadOrStore(
		measurement.Symbol,
		newSymbolState(story.bufferSize),
	)

	state := raw.(*symbolState)

	complete, err := state.absorb(measurement)

	if err != nil {
		return err
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

	evaluation := evaluationPool.Get().(*logic.Evaluation)
	defer evaluationPool.Put(evaluation)

	if evaluation, state.walk, err = story.tree.EvaluateContinuing(
		ordered,
		story.holdings,
		state.walk,
		walkTrace,
		&thresholdCtx,
	); errnie.Error(err) != nil {
		return errnie.Err(
			errnie.Validation,
			"story: failed to evaluate playbook",
			err,
		)
	}

	story.playbookEvaluations++

	story.bus.Send(internal.ChannelUI, "state", map[string]any{
		"measurements":         ordered,
		"gauge_readings":       gaugeReadings,
		"walk":                 state.walk,
		"story_ticks":          story.storyTicks,
		"playbook_evaluations": story.playbookEvaluations,
		"decision_walk":        walkTrace,
		"regime":               story.regime,
	})

	if evaluation != nil && evaluation.Action != nil {
		action := evaluation.Action

		if action.Symbol == "" {
			stamped := *action
			stamped.Symbol = measurement.Symbol
			action = &stamped
			evaluation.Action = action
		}

		prepared, err := prepareAction(
			story.holdings,
			action,
			ordered,
			story.tradingConfig,
			story.paperWalletQuote,
		)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"story: failed to prepare action",
				err,
			))
		}

		if prepared == nil {
			return nil
		}

		action = prepared
		evaluation.Action = action
		state.walk = nil

		return rawbus.Send(story.bus, rawbus.TypeActions, action)
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
