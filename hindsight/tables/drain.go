package tables

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/data"
)

// Drain persists owned observations. Complete training publications feed the
// cold learner; its resolved outcomes are persisted through the same writer.
func (catalog *Catalog) Drain(
	ctx context.Context,
	epoch int64,
	tee *hindsight.StoreTee,
	learn ...func(*data.Measurement[float64]) ([]ExcursionRecord, error),
) error {
	if catalog == nil || tee == nil {
		return nil
	}

	writer := NewWriter(catalog, epoch)

	// These are storage batching cadences, not market observation horizons.
	flushTicker := time.NewTicker(50 * time.Millisecond)
	defer flushTicker.Stop()
	commitTicker := time.NewTicker(30 * time.Second)
	defer commitTicker.Stop()

	drain := func() error {
		// Bound each batch by the observations already waiting, so continuous
		// ingress cannot postpone commits indefinitely.
		for remaining := tee.Pending(); remaining > 0; remaining-- {
			measurement := (*data.Measurement[float64])(tee.Next())

			if measurement == nil {
				return nil
			}

			if measurement.SeqIdx <= 0 {
				return errnie.Error(errnie.Err(errnie.Validation, "catalog: observation has no workspace sequence", nil))
			}

			if measurement.Source == "training" && len(learn) > 0 {
				records, err := learn[0](measurement)
				if err != nil {
					return err
				}
				for _, record := range records {
					writer.AddExcursion(record)
				}
			}

			writer.Add(deriveChannel(measurement), measurement)
		}

		return nil
	}

	for {
		select {
		case <-ctx.Done():
			if err := drain(); err != nil {
				return err
			}

			return writer.CommitReady(context.WithoutCancel(ctx), true)
		case <-flushTicker.C:
			if err := drain(); err != nil {
				return err
			}

			if err := writer.CommitReady(ctx, false); err != nil {
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
