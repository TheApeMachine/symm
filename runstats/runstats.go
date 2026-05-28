/*
Package runstats exposes a tiny set of free functions that packages
outside trader can call to bump run-level counters without taking a
dependency on trader. The trader registers a sink at startup; until
then every call is a no-op.

This indirection exists so ui/, kraken/client/, and leadlag/ can record
"a frame was dropped", "the websocket reconnected", "the throttle
fired" without dragging the entire trader package into their import
graph. The trader owns the actual counter store; this package is just
the hook.
*/
package runstats

import "sync/atomic"

// Sink is the interface trader implements. Each method is called from
// hot paths so implementations must be allocation-free and lock-free
// (atomic counters are the only sensible backing).
type Sink interface {
	UIFramesSent(n int64)
	UIFramesDropped(n int64)
	UIFramesFiltered(n int64)
	LeadlagThrottle()
	LeadlagRecompute()
	WSConnect()
	WSReconnect()
	TokenRefresh(success bool)
}

var sink atomic.Pointer[Sink]

/*
Install sets the global sink. trader.NewCrypto calls this once at
startup. Tests can swap a fake sink in/out.
*/
func Install(s Sink) {
	sink.Store(&s)
}

func current() Sink {
	if ptr := sink.Load(); ptr != nil {
		return *ptr
	}

	return nil
}

func UIFramesSent(n int64) {
	if s := current(); s != nil {
		s.UIFramesSent(n)
	}
}

func UIFramesDropped(n int64) {
	if s := current(); s != nil {
		s.UIFramesDropped(n)
	}
}

func UIFramesFiltered(n int64) {
	if s := current(); s != nil {
		s.UIFramesFiltered(n)
	}
}

func LeadlagThrottle() {
	if s := current(); s != nil {
		s.LeadlagThrottle()
	}
}

func LeadlagRecompute() {
	if s := current(); s != nil {
		s.LeadlagRecompute()
	}
}

func WSConnect() {
	if s := current(); s != nil {
		s.WSConnect()
	}
}

func WSReconnect() {
	if s := current(); s != nil {
		s.WSReconnect()
	}
}

func TokenRefresh(success bool) {
	if s := current(); s != nil {
		s.TokenRefresh(success)
	}
}
