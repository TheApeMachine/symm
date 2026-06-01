package market

import (
	"bufio"
	"container/ring"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
Story holds the latest playbook verdicts per symbol for dashboards and audits.
*/
type Story struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	ui          *qpool.BroadcastGroup
	buffer      *ring.Ring
	trees       []*perspectives.Tree
	lastGauge   map[string]time.Time
	recorder    *bufio.Writer
}

func NewStory(ctx context.Context, pool *qpool.Q) *Story {
	ctx, cancel := context.WithCancel(ctx)

	story := &Story{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		buffer:      ring.New(128),
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		trees:       make([]*perspectives.Tree, 0),
		lastGauge:   make(map[string]time.Time),
	}

	story.ui = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	fh, err := os.Create(viper.GetViper().GetString("trading.record.file"))

	if err != nil {
		errnie.Error(err)
	}

	story.recorder = bufio.NewWriter(fh)

	for _, channel := range []string{"measurements"} {
		story.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		story.subscribers[channel] = story.broadcasts[channel].Subscribe("measurements", 128)
	}

	return story
}

/*
Tick joins the latest measurements from the perspective signals and publishes them to the story.
*/
func (story *Story) Tick() error {
	var (
		measurement perspectives.Measurement
		ok          bool
	)

	defer story.recorder.Flush()

	for row := range story.subscribers["measurements"].Incoming {
		if row == nil {
			errnie.Warn("nil measurement")
			continue
		}

		if measurement, ok = row.Value.(perspectives.Measurement); !ok {
			errnie.Warn("invalid measurement")
			continue
		}

		raw, err := sonic.Marshal(measurement)

		if err != nil {
			errnie.Error(err)
		}

		fmt.Println(string(raw))
		_, err = story.recorder.Write(append(raw, '\n'))

		if err != nil {
			errnie.Error(err)
		}

		story.recorder.Flush()

		story.buffer.Value = measurement
		story.buffer.Next()

		actions := make([]*perspectives.ActionType, 0)

		for _, tree := range story.trees {
			tree.AddMeasurement(story.buffer.Value.(perspectives.Measurement))

			if action := tree.Action(); action != nil {
				actions = append(actions, action)
			}
		}

		if len(story.trees) == 0 || len(actions) == 0 {
			measurements := make([]perspectives.Measurement, 0)

			story.buffer.Do(func(value any) {
				measurements = append(measurements, value.(perspectives.Measurement))
			})

			story.trees = append(story.trees, errnie.Does(func() (*perspectives.Tree, error) {
				return perspectives.NewTree(story.ctx, measurements)
			}).Or(func(err error) {
				errnie.Error(err)
			}).Value())
		}

		for _, action := range actions {
			story.broadcasts["actions"].Send(&qpool.QValue[any]{Value: action})
		}
	}

	return story.ctx.Err()
}

/*
Close shuts down the story.
*/
func (story *Story) Close() error {
	story.cancel()
	return nil
}
