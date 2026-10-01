package data

import (
	"sync"
	"testing"
)

func TestMeasurementProvenanceConcurrentAccess(t *testing.T) {
	m := NewMeasurement[float64]("websocket", map[string]Metric[float64]{
		"price": {Raw: 100},
		"qty":   {Raw: 1},
	})
	m.SetProvenance("side", "buy")
	m.SetProvenance("channel", "trade")

	peer := NewMeasurement[float64]("websocket", map[string]Metric[float64]{
		"price": {Raw: 101},
		"qty":   {Raw: 2},
	})
	peer.SetProvenance("side", "sell")
	peer.SetProvenance("channel", "trade")

	var wg sync.WaitGroup
	start := make(chan struct{})
	workers := 32

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-start
			for n := 0; n < 200; n++ {
				switch id % 5 {
				case 0:
					m.SetProvenance("side", "buy")
				case 1:
					_, _ = m.GetProvenance("side")
				case 2:
					m.WriteMetric("price", float64(100+n))
				case 3:
					m.Pull(peer, "price", "qty")
				default:
					_ = m.Clone()
					_ = m.MetricsSnapshot()
					_ = m.GetSource()
					m.SetSource("hawkes:trade")
				}
			}
		}(i)
	}

	close(start)
	wg.Wait()
}
