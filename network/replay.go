package network

import (
	"context"
	"sort"
	"sync"
	"time"

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
	ctx    context.Context
	mu     sync.RWMutex
	tape   []*data.Measurement
	queues map[string]*lf.Queue[*data.Measurement]
	store  *store.Client
}

func NewReplayer(ctx context.Context) *Replayer {
	once.Do(func() {
		replayer = &Replayer{
			ctx:    ctx,
			queues: make(map[string]*lf.Queue[*data.Measurement]),
			store:  store.NewClient(ctx, "symm"),
		}

		replayer.System = runtime.NewSystem(ctx, "network.replayer", replayer)

		go func() {
			var loaded []*data.Measurement

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

				loaded = append(loaded, measurement)
			}

			if len(loaded) > 0 {
				replayer.Load(loaded...)
			}
		}()
	})

	return replayer
}

/*
Load appends measurements to the tape, sorts the tape chronologically,
and rebuilds the stream queues.
*/
func (replayer *Replayer) Load(measurements ...*data.Measurement) {
	replayer.mu.Lock()
	defer replayer.mu.Unlock()

	for _, meas := range measurements {
		if meas != nil {
			replayer.tape = append(replayer.tape, meas)
		}
	}

	sort.SliceStable(replayer.tape, func(i, j int) bool {
		if replayer.tape[i].Timestamp == replayer.tape[j].Timestamp {
			return replayer.tape[i].SeqIdx < replayer.tape[j].SeqIdx
		}

		return replayer.tape[i].Timestamp < replayer.tape[j].Timestamp
	})

	replayer.rebuildQueues(0)
}

/*
Reset clears the tape and queues.
*/
func (replayer *Replayer) Reset() {
	replayer.mu.Lock()
	defer replayer.mu.Unlock()

	replayer.tape = nil
	replayer.queues = make(map[string]*lf.Queue[*data.Measurement])
}

/*
Rewind resets the replayer queues to the beginning of the tape
and synchronizes time to the start of history.
*/
func (replayer *Replayer) Rewind() {
	replayer.mu.Lock()
	defer replayer.mu.Unlock()

	replayer.rebuildQueues(0)
}

/*
Seek jumps to the first measurement at or after the given timestamp
and synchronizes the synthetic clock.
*/
func (replayer *Replayer) Seek(timestamp int64) {
	replayer.mu.Lock()
	defer replayer.mu.Unlock()

	idx := sort.Search(len(replayer.tape), func(i int) bool {
		return replayer.tape[i].Timestamp >= timestamp
	})

	replayer.rebuildQueues(idx)
	SetSynthetic(timestamp)
}

/*
FastForward advances synthetic time by duration and jumps playback head.
*/
func (replayer *Replayer) FastForward(d time.Duration) {
	current := time.Now().UnixNano()
	target := current + d.Nanoseconds()
	replayer.Seek(target)
}

func (replayer *Replayer) rebuildQueues(fromIndex int) {
	replayer.queues = make(map[string]*lf.Queue[*data.Measurement])

	for i := fromIndex; i < len(replayer.tape); i++ {
		meas := replayer.tape[i]
		replayer.enqueueLocked(meas)
	}

	if fromIndex >= 0 && fromIndex < len(replayer.tape) {
		SetSynthetic(replayer.tape[fromIndex].Timestamp)
	}
}

func (replayer *Replayer) enqueueLocked(meas *data.Measurement) {
	if meas == nil {
		return
	}

	replayer.getQueueLocked("all").Enqueue(meas)

	if meas.Source != "" {
		replayer.getQueueLocked(meas.Source).Enqueue(meas)
	}

	if meas.Label != "" {
		replayer.getQueueLocked(meas.Label).Enqueue(meas)
	}

	if meas.Source == "spot:trade" {
		replayer.getQueueLocked("trades").Enqueue(meas)
	}

	if meas.Source == "spot:level3" {
		replayer.getQueueLocked("level3").Enqueue(meas)
	}

	if meas.Source == "balance" {
		replayer.getQueueLocked("balances").Enqueue(meas)
	}

	if meas.Source == "balances" {
		replayer.getQueueLocked("balance").Enqueue(meas)
	}

	if meas.Source == "orders" {
		replayer.getQueueLocked("order").Enqueue(meas)
	}

	if meas.Source == "executions" {
		replayer.getQueueLocked("execution").Enqueue(meas)
	}
}

func (replayer *Replayer) getQueueLocked(key string) *lf.Queue[*data.Measurement] {
	if q, ok := replayer.queues[key]; ok {
		return q
	}

	q := lf.NewQueue[*data.Measurement]()
	replayer.queues[key] = q
	return q
}

func (replayer *Replayer) Read(keys ...string) ([]byte, error) {
	if len(keys) == 0 {
		keys = []string{"all"}
	}

	for _, key := range keys {
		replayer.mu.RLock()
		q, ok := replayer.queues[key]
		replayer.mu.RUnlock()

		if !ok || q == nil {
			continue
		}

		measurement, found := q.Dequeue()

		if !found || measurement == nil {
			continue
		}

		if measurement.Timestamp > 0 {
			SetSynthetic(measurement.Timestamp)
		}

		return measurement.MarshalBinary()
	}

	return nil, errnie.Error(errnie.Err(
		errnie.NotFound, "[network.replayer] no data found", nil,
	))
}

func (replayer *Replayer) Write(data []byte) error {
	return nil
}
