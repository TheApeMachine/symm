package tables

import (
	"context"
	"fmt"
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
	defer writer.ReleaseRemaining()

	// These are storage batching cadences, not market observation horizons.
	flushTicker := time.NewTicker(50 * time.Millisecond)
	defer flushTicker.Stop()
	commitTicker := time.NewTicker(30 * time.Second)
	defer commitTicker.Stop()

	drain := func() error {
		// Bound each batch by the observations already waiting, so continuous
		// ingress cannot postpone commits indefinitely.
		for remaining := tee.Pending(); remaining > 0; remaining-- {
			pub := data.To[*data.Publication](tee.Next())

			if pub == nil || pub.Measurement == nil {
				return nil
			}

			measurement := pub.Measurement

			if measurement.SeqIdx <= 0 {
				pub.Release()

				errnie.Error(errnie.Err(
					errnie.Validation,
					fmt.Sprintf("[catalog] observation (source=%s, label=%s, id=%d, seq=%d, at=%v) has no workspace sequence", measurement.Source, measurement.Label, measurement.ID, measurement.SeqIdx, measurement.At),
					nil,
				))

				continue
			}

			if val, ok := measurement.GetMetadata("excursion"); ok && val != "" && len(learn) > 0 {
				records, err := learn[0](measurement)

				if err != nil {
					pub.Release()

					errnie.Error(errnie.Err(
						errnie.Validation,
						fmt.Sprintf("[catalog] observation (source=%s, label=%s, id=%d, seq=%d, at=%v) has no workspace sequence", measurement.Source, measurement.Label, measurement.ID, measurement.SeqIdx, measurement.At),
						nil,
					))

					continue
				}

				for _, record := range records {
					writer.AddExcursion(record)
				}
			}

			writer.Add(deriveChannel(measurement), *pub)
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

/*
deriveChannel routes a measurement into its Iceberg family.

Venue tape tables (SpotTicker/Trade/Level3, futures) accept ONLY raw ingress
snapshots whose Source is still "websocket". Signal producers Fork the ingress,
SetSource to e.g. liquidity:ticker, and keep provenance channel=ticker — without
an immutable ingress identity those snapshots polluted SpotTicker.

Prefer provenance ingress_channel (set once at websocket ingress and never
rewritten by stage consumers). Fall back to channel/type only for raw sources.
*/
func deriveChannel(measurement *data.Measurement[float64]) string {
	if measurement == nil {
		return "measurements"
	}

	source := measurement.GetSource()
	// Signal / solver publications always land in Measurements.
	if source != "" && source != "websocket" {
		return "measurements"
	}

	channel := ""
	if value, ok := measurement.GetProvenance("ingress_channel"); ok {
		channel = value
	}
	if channel == "" {
		if value, ok := measurement.GetProvenance("channel"); ok {
			channel = value
		}
	}
	if channel == "" {
		if value, ok := measurement.GetMetadata("type"); ok {
			channel = value
		}
	}

	switch channel {
	case "ticker", "trade", "level3", "futures_ticker", "futures_trade":
		return channel
	}

	// Legacy publishers that only set venue=true + provenance channel.
	if val, ok := measurement.GetMetadata("venue"); ok && val == "true" {
		if value, ok := measurement.GetProvenance("channel"); ok {
			switch value {
			case "ticker", "trade", "level3":
				return value
			}
		}
	}

	return "measurements"
}
