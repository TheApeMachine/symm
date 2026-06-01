package optimizer

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
Tuner consumes replay measurements and stops when the parent context is canceled.
*/
type Tuner struct {
	ctx              context.Context
	cancel           context.CancelFunc
	pool             *qpool.Q
	broadcasts       map[string]*qpool.BroadcastGroup
	subscribers      map[string]*qpool.Subscriber
	measurementCount atomic.Uint64
}

/*
NewTuner creates a new Tuner.
*/
func NewTuner(ctx context.Context, pool *qpool.Q) *Tuner {
	ctx, cancel := context.WithCancel(ctx)

	tuner := &Tuner{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
	}

	for _, channel := range []string{"measurements"} {
		tuner.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		tuner.subscribers[channel] = tuner.broadcasts[channel].Subscribe("optimizer:tuner", 128)
	}

	return tuner
}

/*
Tick drains measurements until the replay session ends.
*/
func (tuner *Tuner) Tick() error {
	for {
		select {
		case <-tuner.ctx.Done():
			return tuner.ctx.Err()
		case row, ok := <-tuner.subscribers["measurements"].Incoming:
			if !ok {
				return nil
			}

			if row == nil {
				continue
			}

			if _, ok := row.Value.(perspectives.Measurement); !ok {
				continue
			}

			tuner.measurementCount.Add(1)
		}
	}
}

/*
MeasurementCount returns how many measurements were observed in the current session.
*/
func (tuner *Tuner) MeasurementCount() uint64 {
	return tuner.measurementCount.Load()
}

/*
Close shuts down the tuner.
*/
func (tuner *Tuner) Close() error {
	tuner.cancel()

	return nil
}

/*
SessionSummary is the optimizer output for one replay pass.
*/
type SessionSummary struct {
	MeasurementCount uint64 `json:"measurement_count"`
}

/*
Summary reports the current session counters.
*/
func (tuner *Tuner) Summary() SessionSummary {
	return SessionSummary{
		MeasurementCount: tuner.MeasurementCount(),
	}
}

/*
String formats the session summary for stderr.
*/
func (summary SessionSummary) String() string {
	return fmt.Sprintf("measurements=%d", summary.MeasurementCount)
}
