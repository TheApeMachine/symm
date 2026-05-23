package engine

import (
	"context"
	"iter"
	"time"
)

/*
Scanner runs a periodic scan loop and drains measurements from a shared queue.
*/
type Scanner struct {
	ctx      context.Context
	cancel   context.CancelFunc
	interval time.Duration
	queue    *MeasurementQueue
}

/*
NewScanner creates a bounded scan runner bound to parent context cancellation.
*/
func NewScanner(parent context.Context, interval time.Duration) *Scanner {
	ctx, cancel := context.WithCancel(parent)

	if interval <= 0 {
		interval = 100 * time.Millisecond
	}

	return &Scanner{
		ctx:      ctx,
		cancel:   cancel,
		interval: interval,
		queue:    &MeasurementQueue{},
	}
}

/*
Run starts the scan loop on a background goroutine.
*/
func (scanner *Scanner) Run(scan func(time.Time)) {
	go func() {
		ticker := time.NewTicker(scanner.interval)
		defer ticker.Stop()

		for {
			select {
			case <-scanner.ctx.Done():
				return
			case tick := <-ticker.C:
				scan(tick)
			}
		}
	}()
}

/*
Measure yields queued measurements for the trader.
*/
func (scanner *Scanner) Measure(ctx context.Context) iter.Seq[Measurement] {
	return scanner.queue.Drain(ctx)
}

/*
Close stops the scan loop.
*/
func (scanner *Scanner) Close() error {
	scanner.cancel()

	return nil
}

/*
Enqueue stores one measurement in the scanner queue.
*/
func (scanner *Scanner) Enqueue(measurement Measurement) {
	scanner.queue.Enqueue(measurement)
}
