package resonance

import (
	"math"
	"sync"

	"github.com/theapemachine/nomagique/statistic"
)

type scalarRing struct {
	samples []float64
}

func (ring *scalarRing) observe(value float64) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return
	}

	ring.samples = append(ring.samples, value)
	capacity := ringCapacity(ring.samples)

	if capacity <= 0 || len(ring.samples) <= capacity {
		return
	}

	ring.samples = ring.samples[len(ring.samples)-capacity:]
}

func ringCapacity(samples []float64) int {
	sampleCount := len(samples)

	if sampleCount < 3 {
		return sampleCount + 1
	}

	span := statistic.SpanOf(samples)

	if span <= 0 {
		return sampleCount + 1
	}

	capacity := int(span)

	if capacity < sampleCount {
		return sampleCount
	}

	return capacity
}

func ratioToMedian(value float64, ring *scalarRing) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 0
	}

	ring.observe(value)
	median := statistic.MedianOf(ring.samples)

	if median <= 1e-12 {
		return 1
	}

	return value / median
}

func scaledSigned(value float64, ring *scalarRing) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}

	ring.observe(value)
	scale := statistic.MedianAbsoluteOf(ring.samples)

	if scale <= 1e-12 {
		return value
	}

	return value / scale
}

type symbolBaselines struct {
	changeAbs     scalarRing
	spreadBps     scalarRing
	logVolume     scalarRing
	tradeRate     scalarRing
	tradeNotional scalarRing
	touchImbal    scalarRing
	depthImbal    scalarRing
	buyPressure   scalarRing
	spreadWide    scalarRing
	tickCadence   scalarRing
	midDrift      scalarRing
}

type senseRegistry struct {
	symbols sync.Map
}

func newSenseRegistry() *senseRegistry {
	return &senseRegistry{}
}

func (registry *senseRegistry) baselines(symbol string) *symbolBaselines {
	if raw, ok := registry.symbols.Load(symbol); ok {
		return raw.(*symbolBaselines)
	}

	created := &symbolBaselines{}
	actual, loaded := registry.symbols.LoadOrStore(symbol, created)

	if loaded {
		return actual.(*symbolBaselines)
	}

	return created
}
