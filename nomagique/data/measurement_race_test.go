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
	peer.SetMetric("bid", NewMetric[float64]("bid", UnitPrice, TimescaleInstantaneous, 65000.5, 1.0).Write(65000.0))
	peer.SetMetric("ask", NewMetric[float64]("ask", UnitPrice, TimescaleInstantaneous, 65000.5, 1.0).Write(65001.0))
	peer.SetProvenance("channel", "ticker")

	m := NewMeasurement[float64]("liquidity")
	m.Label = "BTC/USD"
	m.SeqIdx = 42
	m.Peers = []*Measurement[float64]{peer}
	m.SetMetric("relative_spread", NewMetric[float64]("relative_spread", UnitRelativeSpread, TimescaleInstantaneous, 0.0001, 0.0001).Write(0.0001))
	m.SetMetric("midpoint", NewMetric[float64]("midpoint", UnitPrice, TimescaleInstantaneous, 65000.5, 1.0).Write(65000.5))
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
