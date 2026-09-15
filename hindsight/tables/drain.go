package tables

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Drain consumes *data.Measurement[float64] from the workspace ring buffer,
routes each measurement to its canonical Iceberg table family, and commits snapshots
using per-family batch thresholds and a periodic commit cadence.
It blocks until cancellation and the final drain complete.
*/
func (catalog *Catalog) Drain(
	ctx context.Context,
	epoch int64,
	deps ...any,
) {
	if catalog == nil || catalog.storeTee == nil {
		return
	}

	var fragmentSink func([][]*data.Measurement[float64])
	var groundTruthSink func(ExcursionRecord)

	for _, dep := range deps {
		switch dependency := dep.(type) {
		case func([][]*data.Measurement[float64]):
			fragmentSink = dependency
		case func(ExcursionRecord):
			groundTruthSink = dependency
		}
	}

	writer := NewWriter(catalog, epoch)
	detector := NewStreamingDetector(epoch, 200.0, func(record ExcursionRecord) {
		writer.AddExcursion(record)

		if groundTruthSink != nil {
			groundTruthSink(record)
		}
	}, fragmentSink)

	flushTicker := time.NewTicker(50 * time.Millisecond)
	defer flushTicker.Stop()

	commitTicker := time.NewTicker(30 * time.Second)
	defer commitTicker.Stop()

	var tickCounter atomic.Int64

	for {
		select {
		case <-ctx.Done():
			for {
				measurement := (*data.Measurement[float64])(catalog.storeTee.Next())

				if measurement == nil {
					break
				}

				if measurement.SeqIdx <= 0 {
					measurement.SeqIdx = tickCounter.Add(1)
				}

				detector.Process(measurement)

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
				measurement := (*data.Measurement[float64])(catalog.storeTee.Next())

				if measurement == nil {
					break
				}

				if measurement.SeqIdx <= 0 {
					measurement.SeqIdx = tickCounter.Add(1)
				}

				detector.Process(measurement)

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
