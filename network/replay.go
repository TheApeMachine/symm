package network

import (
	"context"
	"sync"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"golang.design/x/lockfree/lf"
)

var (
	replayer *Replayer
	once     sync.Once
)

type Replayer struct {
	*runtime.System
	ctx    context.Context
	store  *store.Client
	queues map[string]*lf.Queue[*data.Measurement]
}

func NewReplayer(ctx context.Context) *Replayer {
	once.Do(func() {
		replayer = &Replayer{
			ctx:   ctx,
			store: store.NewClient(ctx, "symm"),
			queues: map[string]*lf.Queue[*data.Measurement]{
				"spot:trade":  lf.NewQueue[*data.Measurement](),
				"spot:level3": lf.NewQueue[*data.Measurement](),
			},
		}

		replayer.System = runtime.NewSystem(ctx, "network.replayer", replayer)

		go func() {
			for measurement := range replayer.store.Get(
				"spot:trade", "spot:level3",
			) {
				select {
				case <-ctx.Done():
					return
				default:
				}

				replayer.queues[measurement.Source].Enqueue(measurement)
			}
		}()
	})

	return replayer
}

func (replayer *Replayer) Read(keys ...string) ([]byte, error) {
	if len(keys) > 0 {
		measurement, found := replayer.queues[keys[0]].Dequeue()

		if !found || measurement == nil {
			return nil, nil
		}

		if measurement.Timestamp > 0 {
			SetSynthetic(measurement.Timestamp)
		}

		return measurement.MarshalBinary()
	}

	return nil, nil
}

func (replayer *Replayer) Write(data []byte) error {
	return nil
}
