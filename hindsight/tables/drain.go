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
			ptr := tee.Next()

			if ptr == nil {
				continue
			}

			pub := data.To[data.Publication](ptr)

			if pub.Measurement == nil {
				continue
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

			writer.Add(Measurements, pub)
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
