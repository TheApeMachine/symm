package types

import (
	"sync"
	"testing"
	"time"
)

/*
TestThesisPublishRaceStress proves concurrent Publish and SnapshotMeasurements
do not race under the race detector.
*/
func TestThesisPublishRaceStress(t *testing.T) {
	thesis := NewThesis()
	at := time.Unix(1, 0).UTC()
	waitGroup := sync.WaitGroup{}

	for writerIndex := range 8 {
		waitGroup.Add(1)

		go func(index int) {
			defer waitGroup.Done()

			for publishIndex := range 128 {
				thesis.Publish(SourceHawkes, []*Measurement{{
					Source: SourceHawkes,
					Metric: MetricEventCount,
					Symbol: "BTC/USD",
					Side:   SideBuy,
					Raw:    float64(index*128 + publishIndex),
					At:     at,
				}})
			}
		}(writerIndex)
	}

	for range 8 {
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			for range 128 {
				_ = thesis.SnapshotMeasurements()
				thesis.AppendMeasurements(nil)
			}
		}()
	}

	waitGroup.Wait()
}
