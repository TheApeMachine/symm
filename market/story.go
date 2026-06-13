package market

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/rawbus"
	"github.com/theapemachine/symm/trader"
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
	capitalProvider     trader.CapitalProvider
	pendingIntents      *sync.Map
	accountSyncStarted  atomic.Bool
	bufferSize          int
	storyTicks          atomic.Int64
	playbookEvaluations atomic.Int64
	recorder            *audit.Recorder
	confidenceBaseline  *Baseline
}

type GaugeReading struct {
	Chart            string             `json:"chart"`
	Source           logic.SourceType   `json:"source"`
	Symbol           string             `json:"symbol"`
	Confidence       float64            `json:"confidence"`
	Surprise         float64            `json:"surprise"`
	Strength         float64            `json:"strength"`
	Elapsed          float64            `json:"elapsed"`
	Category         logic.CategoryType `json:"category"`
	ObservedAt       time.Time          `json:"observed_at"`
	ActiveReadings   int                `json:"active_readings"`
	ReadingsCapacity int                `json:"readings_capacity"`
	Calibrating      bool               `json:"calibrating"`
	Calibrated       bool               `json:"calibrated"`
	BestEffort       bool               `json:"best_effort,omitempty"`
	GapReason        string             `json:"gap_reason,omitempty"`
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
			internal.Subscribe(
				internal.ChannelRaw,
				"story:account",
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

	capitalProvider, err := trader.NewCapitalProvider(tradingConfig)

	if err != nil {
		cancel()
		return nil, errnie.Error(err)
	}

	var recorder *audit.Recorder

	if viper.GetBool("system.audit.enabled") {
		recorder, err = audit.NewRecorder(viper.GetString("system.audit.file"))

		if err != nil {
			cancel()
			return nil, errnie.Error(err)
		}
	}

	story := &Story{
		ctx:             ctx,
		cancel:          cancel,
		pool:            pool,
		bus:             bus,
		symbols:         &sync.Map{},
		holdings:        logic.NewHoldings(),
		tree:            tree,
		crossSection:    crossSection,
		regime:          regime,
		tradingConfig:   tradingConfig,
		capitalProvider: capitalProvider,
		pendingIntents:  &sync.Map{},
		bufferSize:      bufferSize,
		recorder:        recorder,
		// The entry confidence bar is DERIVED from the live distribution of
		// signal confidences, not a fixed constant: a signal must stand out
		// against what the system is actually producing right now. The config
		// baseline only seeds warmup and bounds the result.
		confidenceBaseline: NewBaseline(confidenceBaselineFloor, confidenceBaselineMinObs),
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

	story.startAccountSync()

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

	story.observeConfidence(measurement.Confidence)

	complete, err := state.absorb(measurement)

	if err != nil {
		return err
	}

	if complete {
		state.appendSpectrum()
		state.resetSpectrum()
		story.storyTicks.Add(1)
	}

	ordered := state.orderedMeasurements()
	gaugeReadings := state.slotMeasurements()
	decisionMeasurements := state.decisionMeasurements(
		measurement.ObservedAt,
		story.decisionEvidenceTTL(),
	)
	gaugeWireReadings := story.gaugeWireReadings(gaugeReadings)

	walkTrace := &logic.WalkTrace{Symbol: measurement.Symbol}
	thresholdCtx := story.thresholdContext()

	evaluation := evaluationPool.Get().(*logic.Evaluation)
	defer evaluationPool.Put(evaluation)

	if evaluation, state.walk, err = story.tree.EvaluateContinuing(
		decisionMeasurements,
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

	story.playbookEvaluations.Add(1)

	if evaluation == nil || evaluation.Action == nil {
		if err := story.recordNoActionTrace(walkTrace); err != nil {
			return errnie.Error(err)
		}
	}

	story.bus.Send(internal.ChannelUI, "state", map[string]any{
		"measurements":         ordered,
		"gauge_readings":       gaugeWireReadings,
		"walk":                 state.walk,
		"story_ticks":          story.storyTicks.Load(),
		"playbook_evaluations": story.playbookEvaluations.Load(),
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

		if story.hasPendingIntent(action) {
			return nil
		}

		prepared, err := prepareAction(
			story.ctx,
			story.holdings,
			action,
			decisionMeasurements,
			story.tradingConfig,
			story.tree.ThresholdConfig(),
			story.capitalProvider,
			story.regimeVolatility(),
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
		story.ensureActionIDs(action)
		evaluation.Action = action
		state.walk = nil

		if err := story.submitAction(action); err != nil {
			return errnie.Error(err)
		}

		return nil
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
