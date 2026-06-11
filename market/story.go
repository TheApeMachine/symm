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
	holdings     *logic.Holdings
	tree         *logic.Tree
	crossSection *CrossSection
	regime       *RegimeClassifier
	bufferSize   int
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
		holdings:     logic.NewHoldings(),
		tree:         tree,
		crossSection: crossSection,
		regime:       regime,
		bufferSize:   viper.GetInt("story.measurements.buffer"),
	}
}

/*
Tick joins measurements from perspective signals, evaluates the playbook, and
streams regime and decision-tree frames to the UI as measurements arrive.
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

		if row, err = story.bus.Receive("measurements"); errnie.Error(err) != nil {
			return err
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

			story.holdings.SetQuantity(action.Symbol, action.Quantity)
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

			raw, _ := story.measurements.LoadOrStore(measurement.Symbol, ring.New(
				story.bufferSize,
			))

			measurements := raw.(*ring.Ring)
			measurements.Value = measurement
			measurements = measurements.Next()

			var evaluation *logic.Evaluation

			ordered := make([]logic.Measurement, 0, measurements.Len())

			measurements.Move(1).Do(func(item any) {
				if item == nil {
					return
				}

				ordered = append(ordered, item.(logic.Measurement))
			})

			measurements = measurements.Move(story.bufferSize - 1)

			if len(ordered) > 0 {
				evaluation, err = story.tree.Evaluate(ordered, story.holdings)

				if errnie.Error(err) != nil {
					return errnie.Error(err)
				}
			}

			if evaluation == nil || evaluation.Action == nil {
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
		}
	}
}

/*
Close shuts down the story.
*/
func (story *Story) Close() error {
	story.cancel()
	return story.bus.Close()
}
