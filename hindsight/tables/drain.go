package tables

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

// Drain persists owned observations and passes completed, paired excursions and
// tape fragments to the learner. A persistence or learning failure is returned.
func (catalog *Catalog) Drain(
	ctx context.Context,
	epoch int64,
	learn ...func(ExcursionRecord, [][]*data.Measurement[float64]) error,
) error {
	if catalog == nil || catalog.storeTee == nil {
		return nil
	}

	writer := NewWriter(catalog, epoch)
	var completed ExcursionRecord
	var err error
	detector := NewStreamingDetector(epoch, 200.0, func(record ExcursionRecord) {
		writer.AddExcursion(record)
		completed = record
	})

	if len(learn) > 0 {
		detector.SetFragmentSink(func(frames [][]*data.Measurement[float64]) {
			err = learn[0](completed, frames)
		})
	}

	// These are storage batching cadences, not market observation horizons.
	flushTicker := time.NewTicker(50 * time.Millisecond)
	defer flushTicker.Stop()
	commitTicker := time.NewTicker(30 * time.Second)
	defer commitTicker.Stop()

	drain := func() error {
		for {
			measurement := (*data.Measurement[float64])(catalog.storeTee.Next())

			if measurement == nil {
				return nil
			}

			if measurement.SeqIdx <= 0 {
				return errnie.Error(errnie.Err(errnie.Validation, "catalog: observation has no workspace sequence", nil))
			}

			if measurement.Metadata["venue"] == "true" && measurement.Provenance["channel"] != "executions" {
				detector.Process(measurement)
			}

			if err != nil {
				return errnie.Error(err)
			}

			writer.Add(deriveChannel(measurement), measurement)

			if err := writer.CommitReady(ctx, false); err != nil {
				return err
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return writer.CommitReady(context.Background(), true)
		case <-flushTicker.C:
			if err := drain(); err != nil {
				return err
			}
		case <-commitTicker.C:
			if err := writer.CommitReady(ctx, true); err != nil {
				return err
			}
		}
	}
}

func deriveChannel(measurement *data.Measurement[float64]) string {
	if measurement.Metadata["venue"] == "true" {
		switch channel := measurement.Provenance["channel"]; channel {
		case "ticker", "trade", "level3":
			return channel
		}
	}

	return "measurements"
}
