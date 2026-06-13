package telemetry

import (
	"math"
	"sort"
	"sync"
	"sync/atomic"
)

/*
SurpriseIndex aggregates per-source surprise/threshold ratios into one market
chaos gauge that modulates adaptive baseline forgetting rates.
*/
type SurpriseIndex struct {
	ratios      sync.Map
	cachedIndex atomic.Uint64
	cacheValid  atomic.Bool
}

func NewSurpriseIndex() *SurpriseIndex {
	return &SurpriseIndex{}
}

func (index *SurpriseIndex) Record(source string, surprise, threshold float64) {
	if source == "" || surprise <= 0 || threshold <= 0 {
		return
	}

	ratioBits := math.Float64bits(surprise / threshold)
	index.ratios.Store(source, ratioBits)
	index.cacheValid.Store(false)
}

func (index *SurpriseIndex) Index() float64 {
	if index.cacheValid.Load() {
		return math.Float64frombits(index.cachedIndex.Load())
	}

	values := index.ratioValues()

	if len(values) == 0 {
		index.cachedIndex.Store(math.Float64bits(1))
		index.cacheValid.Store(true)

		return 1
	}

	sort.Float64s(values)

	middle := len(values) / 2

	var median float64

	if len(values)%2 == 1 {
		median = values[middle]
	} else {
		median = (values[middle-1] + values[middle]) / 2
	}

	index.cachedIndex.Store(math.Float64bits(median))
	index.cacheValid.Store(true)

	return median
}

func (index *SurpriseIndex) Reset() {
	index.ratios.Range(func(key, value any) bool {
		index.ratios.Delete(key)

		return true
	})

	index.cacheValid.Store(false)
}

func (index *SurpriseIndex) SnapshotRatios() map[string]float64 {
	snapshot := make(map[string]float64)

	index.ratios.Range(func(key, value any) bool {
		source, sourceOK := key.(string)
		ratioBits, ratioOK := value.(uint64)

		if sourceOK && ratioOK {
			snapshot[source] = math.Float64frombits(ratioBits)
		}

		return true
	})

	if len(snapshot) == 0 {
		return nil
	}

	return snapshot
}

func (index *SurpriseIndex) RestoreRatios(ratios map[string]float64) {
	index.Reset()

	for source, ratio := range ratios {
		index.ratios.Store(source, math.Float64bits(ratio))
	}

	index.cacheValid.Store(false)
}

func (index *SurpriseIndex) ratioValues() []float64 {
	values := make([]float64, 0)

	index.ratios.Range(func(key, value any) bool {
		ratioBits, ratioOK := value.(uint64)

		if ratioOK {
			values = append(values, math.Float64frombits(ratioBits))
		}

		return true
	})

	return values
}

var sharedSurpriseIndex atomic.Pointer[SurpriseIndex]

func init() {
	sharedSurpriseIndex.Store(NewSurpriseIndex())
}

func SharedSurpriseIndex() *SurpriseIndex {
	return sharedSurpriseIndex.Load()
}

func RecordSurpriseRatio(source string, surprise, threshold float64) {
	SharedSurpriseIndex().Record(source, surprise, threshold)
}

func MarketSurpriseIndex() float64 {
	index := SharedSurpriseIndex().Index()

	if math.IsNaN(index) || math.IsInf(index, 0) || index <= 0 {
		return 1
	}

	return index
}
