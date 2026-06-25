package resonance

import (
	"math"
	"sync"

	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/statutil"
)

type scalarRing struct {
	samples []float64
	stamps  []float64
}

func (ring *scalarRing) observe(value float64, stamp float64) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return
	}

	if stamp <= 0 {
		return
	}

	ring.samples = append(ring.samples, value)
	ring.stamps = append(ring.stamps, stamp)
	keep := statutil.WindowDepth(ring.stamps)

	if keep <= 0 || len(ring.samples) <= keep {
		return
	}

	ring.samples = ring.samples[len(ring.samples)-keep:]
	ring.stamps = ring.stamps[len(ring.stamps)-keep:]
}

func spreadRatioToMedian(value float64, ring *scalarRing, stamp float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 0
	}

	if len(ring.samples) == 0 {
		ring.observe(value, stamp)

		if len(ring.samples) == 0 {
			return 0
		}

		return 1
	}

	return ratioToMedian(value, ring, stamp)
}

func ratioToMedian(value float64, ring *scalarRing, stamp float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 0
	}

	ring.observe(value, stamp)
	median, medianOK := statistic.MedianOf(ring.samples)

	if !medianOK || median <= 0 {
		return 1
	}

	return value / median
}

func scaledSigned(value float64, ring *scalarRing, stamp float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}

	ring.observe(value, stamp)
	scale, scaleOK := statistic.MedianAbsoluteOf(ring.samples)

	if !scaleOK || scale <= 0 {
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
