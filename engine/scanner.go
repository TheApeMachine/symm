package engine

import (
	"context"
	"iter"
)

/*
Scanner holds a bounded measurement queue for one signal.
The trader scheduler calls Scan on each signal, then drains through Measure.
*/
type Scanner struct {
	ctx    context.Context
	cancel context.CancelFunc
	queue  *MeasurementQueue
}

/*
NewScanner creates a passive queue bound to parent context cancellation.
*/
func NewScanner(parent context.Context) *Scanner {
	ctx, cancel := context.WithCancel(parent)

	return &Scanner{
		ctx:    ctx,
		cancel: cancel,
		queue:  &MeasurementQueue{},
	}
}

/*
Measure yields queued measurements for the trader.
*/
func (scanner *Scanner) Measure(ctx context.Context) iter.Seq[Measurement] {
	return scanner.queue.Drain(ctx)
}

/*
Stats returns live queue counters.
*/
func (scanner *Scanner) Stats() QueueStats {
	return scanner.queue.Stats()
}

/*
Close stops the scanner context.
*/
func (scanner *Scanner) Close() error {
	scanner.cancel()

	return nil
}

/*
Enqueue stores one measurement in the scanner queue.
*/
func (scanner *Scanner) Enqueue(measurement Measurement) error {
	return scanner.queue.Enqueue(measurement)
}
