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

const storyMeasurementsSubscriberID = "market:story"

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
	recordFile  *os.File
	recorder    *bufio.Writer
}

func NewStory(ctx context.Context, pool *qpool.Q) (*Story, error) {
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

	recordPath := viper.GetViper().GetString("trading.record.file")

	fh, err := os.Create(recordPath)

	if err != nil {
		cancel()
		return nil, fmt.Errorf("market story: create record file %q: %w", recordPath, err)
	}

	story.recordFile = fh
	story.recorder = bufio.NewWriter(fh)

	for _, channel := range []string{"measurements", "actions"} {
		story.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
	}

	story.subscribers["measurements"] = story.broadcasts["measurements"].Subscribe(
		storyMeasurementsSubscriberID, 128,
	)

	return story, nil
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
		fmt.Println("story.Tick", "measurement", row)

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

		errnie.Error(story.recorder.Flush())

		story.buffer.Value = measurement
		story.buffer.Next()

		actions := make([]*perspectives.ActionType, 0)

		for _, tree := range story.trees {
			if buffered, bufferedOK := story.buffer.Value.(perspectives.Measurement); bufferedOK {
				tree.AddMeasurement(buffered)
			}

			if action := tree.Action(); action != nil {
				actions = append(actions, action)
			}
		}

		if len(story.trees) == 0 || len(actions) == 0 {
			measurements := make([]perspectives.Measurement, 0)

			story.buffer.Do(func(value any) {
				if value == nil {
					return
				}

				buffered, bufferedOK := value.(perspectives.Measurement)

				if !bufferedOK {
					return
				}

				measurements = append(measurements, buffered)
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

	if subscriber := story.subscribers["measurements"]; subscriber != nil {
		if broadcast := story.broadcasts["measurements"]; broadcast != nil {
			broadcast.Unsubscribe(subscriber.ID)
		}
	}

	if story.recorder != nil {
		errnie.Error(story.recorder.Flush())
		story.recorder = nil
	}

	if story.recordFile != nil {
		errnie.Error(story.recordFile.Close())
		story.recordFile = nil
	}

	return nil
}
