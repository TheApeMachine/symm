package tables

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

type DrainSource interface {
	Pending() int
	Pop() *data.Measurement
}

const (
	maxDrainConsecutiveFailures = 10
	maxDrainBufferedBytes       = 256 * 1024 * 1024
	minDrainBackoff             = time.Second
	maxDrainBackoff             = 30 * time.Second
	shutdownFlushTimeout        = 12 * time.Second
)

// Drain persists owned observations. Complete training publications feed the
// cold learner; its resolved outcomes are persisted through the same writer.
func (catalog *Catalog) Drain(
	ctx context.Context,
	epoch int64,
	tee DrainSource,
) error {
	if catalog == nil || tee == nil {
		return nil
	}

	writer := NewWriter(catalog, epoch)
	defer writer.ReleaseRemaining()

	// These are storage batching cadences, not market observation horizons.
	flushTicker := time.NewTicker(50 * time.Millisecond)
	defer flushTicker.Stop()
	commitTicker := time.NewTicker(10 * time.Minute)
	defer commitTicker.Stop()

	drain := func() error {
		// Bound each batch by the observations already waiting, so continuous
		// ingress cannot postpone commits indefinitely.
		for remaining := tee.Pending(); remaining > 0; remaining-- {
			measurement := tee.Pop()

			if measurement == nil {
				continue
			}

			if measurement.Error() != nil {
				continue
			}

			writer.Add(Measurements, measurement)
		}

		return nil
	}

	var consecutiveFailures int
	var backoff time.Duration = minDrainBackoff
	var nextRetry time.Time

	commit := func(forceAll bool) error {
		if !nextRetry.IsZero() && time.Now().Before(nextRetry) {
			return nil
		}

		err := writer.CommitReady(ctx, forceAll)

		if err != nil {
			consecutiveFailures++
			errnie.Warn(fmt.Sprintf(
				"[drain] catalog commit deferred (attempt %d): %s; %d rows (%d bytes) retained",
				consecutiveFailures,
				err,
				writer.BufferedRows(),
				writer.BufferedBytes(),
			))

			if consecutiveFailures >= maxDrainConsecutiveFailures || writer.BufferedBytes() >= maxDrainBufferedBytes {
				return errnie.Error(errnie.Err(
					errnie.IO,
					"[drain] catalog persistence buffer capacity exceeded",
					err,
				))
			}

			if backoff < minDrainBackoff {
				backoff = minDrainBackoff
			}

			backoff = min(backoff*2, maxDrainBackoff)
			nextRetry = time.Now().Add(backoff)

			return nil
		}

		if consecutiveFailures > 0 {
			errnie.Info(fmt.Sprintf(
				"[drain] catalog commit recovered after %d deferred attempts; buffer flushed",
				consecutiveFailures,
			))
		}

		consecutiveFailures = 0
		backoff = minDrainBackoff
		nextRetry = time.Time{}

		return nil
	}

	for {
		select {
		case <-ctx.Done():
			if err := drain(); err != nil {
				return err
			}

			shutdownCtx, cancel := context.WithTimeout(
				context.WithoutCancel(ctx), shutdownFlushTimeout,
			)
			defer cancel()

			return writer.CommitReady(shutdownCtx, true)
		case <-flushTicker.C:
			if err := drain(); err != nil {
				return err
			}

			if err := commit(false); err != nil {
				return err
			}
		case <-commitTicker.C:
			if err := commit(true); err != nil {
				return err
			}
		}
	}
}
