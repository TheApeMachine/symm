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
}

func NewStory(ctx context.Context, pool *qpool.Q[any]) *Story {
	ctx, cancel := context.WithCancel(ctx)

	tree, err := logic.NewTree()

	return &Story{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		bus: internal.NewBus(
			ctx,
			pool,
			[]string{"raw"},
			[]string{"measurements"},
		),
		measurements: &sync.Map{},
		tree:         tree,
		err:          err,
	}
}

/*
Tick joins the latest measurements from the perspective signals and publishes them to the story.

UI events are rate-limited to storyUIInterval. Measurements flood the channel at high frequency
and selecting one "publish" case per measurement would starve the timer and flood the WebSocket.
Instead, we accumulate per-source/symbol readings between UI ticks and flush
cross-sectional means on the timer — then reset the window so each gauge frame
reflects the last interval, not the lifetime of the process.
*/
func (story *Story) Tick() error {
	if story.err != nil {
		return story.err
	}

	for {
		select {
		case <-story.ctx.Done():
			return story.ctx.Err()
		default:
			row, err := story.bus.Receive("measurements")

			if errnie.Error(err) != nil || row == nil {
				continue
			}

			var (
				measurement logic.Measurement
				ok          bool
			)

			if measurement, ok = row.Value.(logic.Measurement); !ok {
				errnie.Error(errors.New("story: invalid measurement"))
				continue
			}

			raw, _ := story.measurements.LoadOrStore(measurement.Symbol, NewMeasurementWindow(
				viper.GetInt("market.story.window_capacity"),
			))

			measurements := raw.(*MeasurementWindow).Push(measurement)

			var action *logic.Action

			if len(measurements) > 0 {
				action = story.tree.Evaluate(measurements)
			}

			if action != nil {
				if action.Symbol == "" {
					stamped := *action
					stamped.Symbol = measurement.Symbol
					action = &stamped
				}

				story.bus.Send("raw", "actions", action)
			}
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
