package public

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLatencyProbeObservePong(t *testing.T) {
	Convey("Given a pending ping request", t, func() {
		latency := NewNetworkLatency()
		probe := newLatencyProbe(latency)

		probe.mu.Lock()
		probe.pending[7] = time.Now().Add(-40 * time.Millisecond)
		probe.mu.Unlock()

		probe.observePong(map[string]any{
			"method": "pong",
			"req_id": float64(7),
		})

		Convey("It should record RTT on the shared latency tracker", func() {
			So(latency.Measured(), ShouldBeTrue)
			So(latency.RTT(), ShouldBeGreaterThan, 30*time.Millisecond)
		})
	})
}

func BenchmarkLatencyProbeObservePong(b *testing.B) {
	latency := NewNetworkLatency()
	probe := newLatencyProbe(latency)

	b.ReportAllocs()

	for b.Loop() {
		index := int(b.N)

		probe.mu.Lock()
		probe.pending[index] = time.Now().Add(-10 * time.Millisecond)
		probe.mu.Unlock()

		probe.observePong(map[string]any{
			"method": "pong",
			"req_id": float64(index),
		})
	}
}
