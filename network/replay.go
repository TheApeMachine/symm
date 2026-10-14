package network

import (
	"context"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"golang.design/x/lockfree/lf"
)

var replayer *Replayer
var once sync.Once

type Replayer struct {
	*runtime.System
	mu     sync.RWMutex
	queues map[string]*lf.Queue[*data.Measurement]
	store  *store.Client
}

func NewReplayer(ctx context.Context) *Replayer {
	once.Do(func() {
		replayer = &Replayer{
			queues: map[string]*lf.Queue[*data.Measurement]{
				"spot:trade":  lf.NewQueue[*data.Measurement](),
				"spot:level3": lf.NewQueue[*data.Measurement](),
				"trades":      lf.NewQueue[*data.Measurement](),
				"level3":      lf.NewQueue[*data.Measurement](),
				"correlation": lf.NewQueue[*data.Measurement](),
				"cvd":         lf.NewQueue[*data.Measurement](),
				"depthflow":   lf.NewQueue[*data.Measurement](),
				"hawkes":      lf.NewQueue[*data.Measurement](),
				"leadlag":     lf.NewQueue[*data.Measurement](),
				"liquidity":   lf.NewQueue[*data.Measurement](),
				"morphology":  lf.NewQueue[*data.Measurement](),
				"pumpdump":    lf.NewQueue[*data.Measurement](),
				"sentiment":   lf.NewQueue[*data.Measurement](),
				"toxicity":    lf.NewQueue[*data.Measurement](),
			},
			store: store.NewClient(ctx, "symm"),
		}

		replayer.System = runtime.NewSystem(ctx, "network.replayer", replayer)

		go func() {
			for buf := range replayer.store.GetBytes("symm") {
				select {
				case <-ctx.Done():
					return
				default:
				}

				measurement := &data.Measurement{}

				if err := measurement.UnmarshalBinary(buf); err != nil {
					errnie.Error(errnie.Err(
						errnie.Validation, "[network.replayer] failed to unmarshal binary", err,
					))

					continue
				}

				if measurement.Timestamp > 0 {
					SetSynthetic(measurement.Timestamp)
				}

				if measurement.Source != "" {
					replayer.getQueue(measurement.Source).Enqueue(measurement)
				}

				if measurement.Source == "spot:trade" {
					replayer.getQueue("trades").Enqueue(measurement)
				}

				if measurement.Source == "spot:level3" {
					replayer.getQueue("level3").Enqueue(measurement)
				}

				if measurement.Label != "" {
					replayer.getQueue(measurement.Label).Enqueue(measurement)
				}
			}
		}()
	})

	return replayer
}

func (replayer *Replayer) getQueue(key string) *lf.Queue[*data.Measurement] {
	replayer.mu.RLock()
	q, ok := replayer.queues[key]
	replayer.mu.RUnlock()

	if ok {
		return q
	}

	replayer.mu.Lock()
	defer replayer.mu.Unlock()

	if q, ok := replayer.queues[key]; ok {
		return q
	}

	q = lf.NewQueue[*data.Measurement]()
	replayer.queues[key] = q
	return q
}

func (replayer *Replayer) Read(keys ...string) ([]byte, error) {
	for _, key := range keys {
		replayer.mu.RLock()
		q, ok := replayer.queues[key]
		replayer.mu.RUnlock()

		if ok && q != nil {
			measurement, found := q.Dequeue()

			if found && measurement != nil {
				if measurement.Timestamp > 0 {
					SetSynthetic(measurement.Timestamp)
				}

				return measurement.MarshalBinary()
			}
		}
	}

	return nil, errnie.Error(errnie.Err(
		errnie.NotFound, "[network.replayer] no data found", nil,
	))
}

func (replayer *Replayer) Write(data []byte) error {
	return nil
}
