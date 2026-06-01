package optimizer

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
Tuner searches for the best tunables and tree YAML.
*/
type Tuner struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	trees       []*perspectives.Tree
}

/*
NewTuner creates a new Tuner.
*/
func NewTuner(ctx context.Context, pool *qpool.Q) (*Tuner, error) {
	ctx, cancel := context.WithCancel(ctx)

	tuner := &Tuner{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
	}

	for _, channel := range []string{"measurements"} {
		tuner.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		tuner.subscribers[channel] = tuner.broadcasts[channel].Subscribe("measurements", 128)
	}

	return tuner, errnie.Error(errnie.Require((map[string]any{
		"ctx":    ctx,
		"cancel": cancel,
		"pool":   pool,
	})))
}

/*
Tick searches for the best tunables and tree YAML.
*/
func (tuner *Tuner) Tick() error {
	for row := range tuner.subscribers["measurements"].Incoming {
		if row == nil {
			continue
		}

		measurement := row.Value.(perspectives.Measurement)
	}

	return tuner.ctx.Err()
}

/*
Project the current trees forwards in time to discover its profitability.
*/
func (tuner *Tuner) Project(tree *perspectives.Tree) float64 {
	return 0
}
