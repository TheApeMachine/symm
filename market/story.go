package market

import (
	"container/ring"
	"context"
	"time"

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
	buffer      *ring.Ring
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
	}

	for _, channel := range []string{"measurements"} {
		story.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		story.subscribers[channel] = story.broadcasts[channel].Subscribe("measurements", 128)
	}

	return story, errnie.Error(errnie.Require((map[string]any{
		"ctx":    ctx,
		"cancel": cancel,
		"pool":   pool,
	})))
}

/*
Tick joins the latest measurements from the perspective signals and publishes them to the story.
*/
func (story *Story) Tick() error {
	var (
		measurement perspectives.Measurement
		ok          bool
	)

	for row := range story.subscribers["measurements"].Incoming {
		if row == nil {
			errnie.Warn("nil measurement")
			continue
		}

		if measurement, ok = row.Value.(perspectives.Measurement); !ok {
			errnie.Warn("invalid measurement")
			continue
		}

		story.buffer.Value = measurement
		story.buffer.Next()

		
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
