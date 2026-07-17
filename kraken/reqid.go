package kraken

import (
	"sync/atomic"
	"time"
)

/*
orderReqID issues monotonic add_order req_id values. UnixNano truncated to int
collides within a nanosecond and is unsafe on 32-bit; this counter starts at
the boot wall clock once and never decreases.
*/
var orderReqID = atomic.Int64{}

func init() {
	orderReqID.Store(time.Now().UnixNano())
}

/*
NextReqID returns the next monotonic request identifier for Kraken WS orders.
*/
func NextReqID() int {
	return int(orderReqID.Add(1))
}
