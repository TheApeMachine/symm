package optimizer

import (
	"sync"

	"github.com/theapemachine/symm/market/perspectives"
)

var replayMeasurementsPool = sync.Pool{
	New: func() any {
		return &replayMeasurements{
			global:  make(map[perspectives.SourceType]perspectives.Measurement, 8),
			symbols: make(map[string]map[perspectives.SourceType]perspectives.Measurement, 4),
		}
	},
}

var replaySnapshotPool = sync.Pool{
	New: func() any {
		buffer := make([]perspectives.Measurement, 0, StoryRingCapacity)

		return &buffer
	},
}

func acquireReplayMeasurements() *replayMeasurements {
	measurements := replayMeasurementsPool.Get().(*replayMeasurements)
	clear(measurements.global)

	for symbol := range measurements.symbols {
		delete(measurements.symbols, symbol)
	}

	return measurements
}

func releaseReplayMeasurements(measurements *replayMeasurements) {
	if measurements == nil {
		return
	}

	replayMeasurementsPool.Put(measurements)
}

func acquireReplaySnapshotBuffer() []perspectives.Measurement {
	buffer := replaySnapshotPool.Get().(*[]perspectives.Measurement)

	return (*buffer)[:0]
}

func releaseReplaySnapshotBuffer(buffer []perspectives.Measurement) {
	if buffer == nil {
		return
	}

	copied := buffer
	replaySnapshotPool.Put(&copied)
}
