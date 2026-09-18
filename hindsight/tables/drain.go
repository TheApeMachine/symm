package tables

import (
	"context"
	"iter"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"golang.design/x/lockfree/lf"
)

/*
Drain is a core.Primitive off-ramp that persists observations to the catalog.
Next enqueues each arriving pointer. An internal goroutine dequeues on a timer
and commits batches to the catalog writer.

TODO: storage schema for map[string]any / grid snapshots.
*/
type Drain struct {
	*core.PrimitiveError
	queue   *lf.Queue[unsafe.Pointer]
	pending atomic.Int64
}

/*
NewDrain constructs a Drain primitive and starts the internal persistence loop.
*/
func NewDrain(
	ctx context.Context,
	catalog *Catalog,
	epoch int64,
) *Drain {
	drain := &Drain{
		PrimitiveError: core.NewPrimitiveError(),
		queue:          lf.NewQueue[unsafe.Pointer](),
	}

	if catalog == nil {
		return drain
	}

	go drain.run(ctx, catalog, epoch)

	return drain
}

/*
Next enqueues each arriving stream item into the lock-free queue for the
internal drain loop. Drain is a terminal off-ramp: it yields nothing.
*/
func (drain *Drain) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				continue
			}

			drain.pending.Add(1)
			drain.queue.Enqueue(arriving)
		}
	}
}

func (drain *Drain) run(
	ctx context.Context,
	catalog *Catalog,
	epoch int64,
) {
	writer := NewWriter(catalog, epoch)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Flush pending items
			drain.flushQueue(writer)
			_ = writer.CommitReady(context.Background(), true)
			return

		case <-ticker.C:
			drain.flushQueue(writer)
			if writer.Pending() > 0 {
				_ = writer.CommitReady(ctx, false)
			}
		}
	}
}

func (drain *Drain) flushQueue(writer *Writer) {
	for {
		item, ok := drain.queue.Dequeue()
		if !ok || item == nil {
			break
		}
		drain.pending.Add(-1)

		// Evaluation from Training System
		eval := (*cognition.Evaluation)(item)
		if eval != nil {
			meas := data.NewMeasurement("training", map[string]data.Metric[float64]{
				"surprisal":  data.NewMetric[float64]("surprisal", data.UnitNat, data.TimescaleInstantaneous, 0, 1).Write(eval.Surprisal),
				"ambiguity":  data.NewMetric[float64]("ambiguity", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1).Write(eval.Ambiguity),
				"confidence": data.NewMetric[float64]("confidence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1).Write(eval.Confidence),
				"contrast":   data.NewMetric[float64]("contrast", data.UnitNat, data.TimescaleInstantaneous, 0, 1).Write(eval.Contrast),
				"support":    data.NewMetric[float64]("support", data.UnitCount, data.TimescaleInstantaneous, 0, 1).Write(float64(eval.Support)),
			})

			meas.SeqIdx = int64(eval.Step)
			meas.At = time.Now().UTC()
			meas.Metadata["winner"] = eval.WinnerClass
			meas.Metadata["context"] = string(eval.Context)

			writer.Add(Measurements, meas)
		}
	}
}
