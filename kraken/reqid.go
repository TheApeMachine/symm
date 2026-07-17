package kraken

import (
	"sync/atomic"
	"time"
)

/*
RequestIDGenerator issues monotonic add_order req_id values. The counter starts
at the boot wall clock once and never decreases, avoiding UnixNano truncation
to int and collisions within a nanosecond.
*/
type RequestIDGenerator struct {
	counter atomic.Int64
}

/*
NewRequestIDGenerator seeds the counter from the current wall clock in Unix
nanoseconds so subsequent Next calls stay monotonic from process boot.
*/
func NewRequestIDGenerator() *RequestIDGenerator {
	generator := &RequestIDGenerator{}
	generator.counter.Store(time.Now().UnixNano())

	return generator
}

/*
Next returns the next monotonic request identifier for Kraken WS orders.
*/
func (generator *RequestIDGenerator) Next() int64 {
	return generator.counter.Add(1)
}

var orderRequestID = NewRequestIDGenerator()
