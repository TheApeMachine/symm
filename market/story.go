package market

import (
	"container/ring"
	"context"

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
			measurement := value.(logic.Measurement)
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
	measurements map[string]*MeasurementWindow
}

func NewStory(ctx context.Context, pool *qpool.Q[any]) *Story {
	ctx, cancel := context.WithCancel(ctx)

	return &Story{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		bus: internal.NewBus(
			ctx,
			pool,
			[]string{"measurements"},
			[]string{},
		),
		measurements: make(map[string]*MeasurementWindow),
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
	for {
		select {
		case <-story.ctx.Done():
			return story.ctx.Err()
		default:
			row, err := story.bus.Receive("measurements")

			if errnie.Error(err) != nil || row == nil {
				continue
			}

			measurement := GetMeasurement(row)

			if _, ok := story.measurements[measurement.Symbol]; !ok {
				story.measurements[measurement.Symbol] = NewMeasurementWindow(
					viper.GetInt("market.story.window.capacity"),
				)
			}

			measurements := story.measurements[measurement.Symbol].Push(measurement)

			if len(measurements) > 0 {
				tree := logic.NewTree()
				tree.Evaluate(measurements)
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
