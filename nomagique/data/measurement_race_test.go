package data

import (
	"sync"
	"testing"
)

/*
Under WORM, once published, Measurement has zero writers and arbitrary concurrent readers.
No mutex or atomics are required.
*/
func TestMeasurementWORMConcurrentReads(t *testing.T) {
	peer := NewMeasurement[float64]("websocket")
	peer.Label = "BTC/USD"
	peer.WriteMetric("bid", 65000.0)
	peer.WriteMetric("ask", 65001.0)
	peer.SetProvenance("channel", "ticker")

	m := NewMeasurement[float64]("liquidity")
	m.Label = "BTC/USD"
	m.SeqIdx = 42
	m.Peers = []*Measurement[float64]{peer}
	m.WriteMetric("relative_spread", 0.0001)
	m.WriteMetric("midpoint", 65000.5)
	m.SetMetadata("support", "10")
	m.SetProvenance("channel", "liquidity")

	var wg sync.WaitGroup
	start := make(chan struct{})
	readers := 32

	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			for n := 0; n < 200; n++ {
				_ = m.GetMetric("relative_spread")
				_ = m.GetMetric("bid")
				_, _ = m.LookupMetric("ask")
				_, _ = m.LookupPeerMetric("websocket", "bid")
				_, _ = m.GetMetadata("support")
				_, _ = m.GetProvenance("channel")
				m.RangeMetrics(func(k string, v Metric[float64]) bool {
					return true
				})
			}
		}()
	}

	close(start)
	wg.Wait()
}
