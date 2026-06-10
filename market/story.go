package market

import (
	"container/ring"
	"context"
	"errors"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/logic"
)

type MeasurementWindow struct {
	ring *ring.Ring
	ptr  int
}

func NewMeasurementWindow(capacity int) *MeasurementWindow {
	return &MeasurementWindow{
		ring: ring.New(capacity),
		ptr:  0,
	}
}

func (window *MeasurementWindow) Push(measurement logic.Measurement) []logic.Measurement {
	window.ring.Value = measurement
	window.ring = window.ring.Next()
	window.ptr++

	measurements := make([]logic.Measurement, 0, window.ring.Len())

	if window.ptr >= window.ring.Len()-1 {
		window.ring.Do(func(value any) {
			if value == nil {
				return
			}

			measurement, ok := value.(logic.Measurement)

			if !ok {
				return
			}

			measurements = append(measurements, measurement)
		})

		window.ring = ring.New(window.ring.Len())
		window.ptr = 0
	}

	return measurements
}

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
	holdings     *logic.Holdings
}

func NewStory(ctx context.Context, pool *qpool.Q[any], holdings *logic.Holdings) *Story {
	ctx, cancel := context.WithCancel(ctx)

	tree, err := logic.NewTree()
	crossSection, crossSectionErr := LoadCrossSection(&CrossSectionOnce{})

	if crossSectionErr != nil {
		err = errors.Join(err, crossSectionErr)
	}

	regime, regimeErr := NewRegimeClassifier(crossSection)

	if regimeErr != nil {
		err = errors.Join(err, regimeErr)
	}

	return &Story{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		bus: internal.NewBus(
			ctx,
			pool,
			[]string{"raw", "ui"},
			[]string{"measurements"},
		),
		measurements: &sync.Map{},
		tree:         tree,
		crossSection: crossSection,
		regime:       regime,
		holdings:     holdings,
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
		if errnie.Error(story.ctx.Err()) != nil {
			return story.ctx.Err()
		}

		row, err := story.bus.Poll("measurements")

		if errnie.Error(err) != nil || row == nil {
			row, err = story.bus.Receive("measurements")
		}

		if errnie.Error(err) != nil {
			if errnie.Error(story.ctx.Err()) != nil {
				return story.ctx.Err()
			}

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
			story.regime.Observe(measurement)
		}

		raw, _ := story.measurements.LoadOrStore(measurement.Symbol, NewMeasurementWindow(
			viper.GetInt("market.story.window_capacity"),
		))

		measurements := raw.(*MeasurementWindow).Push(measurement)

		var evaluation *logic.Evaluation

		if len(measurements) > 0 {
			evaluation = story.tree.Evaluate(measurements, story.holdings)
		}

		if evaluation == nil || evaluation.Action == nil {
			story.publishMarketRegime()
			story.publishDecisionTree()
			continue
		}

		action := evaluation.Action

		if action.Symbol == "" {
			stamped := *action
			stamped.Symbol = measurement.Symbol
			action = &stamped
			evaluation.Action = action
		}

		errnie.Error(story.bus.Send("raw", "actions", action))

		story.publishMarketRegime()
		story.publishDecisionTree()
	}
}

func (story *Story) publishDecisionTree() {
	stats := story.tree.Stats()

	if stats == nil {
		return
	}

	errnie.Error(story.bus.Send("ui", "decision_tree", stats.DecisionTreeFrame()))
}

func (story *Story) publishMarketRegime() {
	if story.regime == nil {
		return
	}

	errnie.Error(story.regime.PublishFrame(story.bus))
}

/*
TreeStats exposes playbook instrumentation for dashboards and decision recording.
*/
func (story *Story) TreeStats() *logic.TreeStats {
	if story.tree == nil {
		return nil
	}

	return story.tree.Stats()
}

/*
Close shuts down the story.
*/
func (story *Story) Close() error {
	story.cancel()
	return story.bus.Close()
}
