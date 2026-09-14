package tables

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"golang.design/x/lockfree/wf"
)

/*
Drain consumes *data.Measurement[float64] from the workspace ring buffer,
routes each measurement to its canonical Iceberg table family, and commits snapshots
using per-family batch thresholds and a periodic commit cadence.
*/
func Drain(
	ctx context.Context,
	catalog *Catalog,
	ring *wf.RingBuffer[*data.Measurement[float64]],
	epoch int64,
) {
	if ring == nil {
		return
	}

	if epoch <= 0 {
		epoch = time.Now().UTC().UnixNano()
	}

	if catalog == nil {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(50 * time.Millisecond):
				for {
					if _, ok := ring.Get(); !ok {
						break
					}
				}
			}
		}
	}

	writer := NewWriter(catalog, epoch)
	flushTicker := time.NewTicker(50 * time.Millisecond)
	defer flushTicker.Stop()

	commitTicker := time.NewTicker(30 * time.Second)
	defer commitTicker.Stop()

	var tickCounter atomic.Int64

	for {
		select {
		case <-ctx.Done():
			for {
				measurement, ok := ring.Get()

				if !ok {
					break
				}

				if measurement == nil {
					continue
				}

				if measurement.SeqIdx <= 0 {
					measurement.SeqIdx = tickCounter.Add(1)
				}

				channel := deriveChannel(measurement)
				writer.Add(channel, measurement)
			}

			if writer.Pending() > 0 {
				if err := writer.CommitReady(context.Background(), true); err != nil {
					errnie.Error(err)
				}
			}

			return

		case <-commitTicker.C:
			if writer.Pending() > 0 {
				if err := writer.CommitReady(ctx, true); err != nil {
					errnie.Error(err)
				}
			}

		case <-flushTicker.C:
			for {
				measurement, ok := ring.Get()

				if !ok {
					break
				}

				if measurement == nil {
					continue
				}

				if measurement.SeqIdx <= 0 {
					measurement.SeqIdx = tickCounter.Add(1)
				}

				channel := deriveChannel(measurement)
				writer.Add(channel, measurement)

				if err := writer.CommitReady(ctx, false); err != nil {
					errnie.Error(err)
				}
			}
		}
	}
}

func deriveChannel(measurement *data.Measurement[float64]) string {
	if measurement.Provenance != nil {
		if channel, ok := measurement.Provenance["channel"]; ok && channel != "" {
			return channel
		}
	}

	if _, hasBid := measurement.Metrics["bid"]; hasBid {
		return "ticker"
	}

	if _, hasPrice := measurement.Metrics["price"]; hasPrice {
		return "trade"
	}

	if _, hasLimit := measurement.Metrics["limit_price"]; hasLimit {
		return "level3"
	}

	return "measurements"
}
