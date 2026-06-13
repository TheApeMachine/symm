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
	ratios sync.Map
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
}

func (index *SurpriseIndex) Index() float64 {
	values := index.ratioValues()

	if len(values) == 0 {
		return 1
	}

	sort.Float64s(values)

	middle := len(values) / 2

	if len(values)%2 == 1 {
		return values[middle]
	}

	return (values[middle-1] + values[middle]) / 2
}

func (index *SurpriseIndex) Reset() {
	index.ratios.Range(func(key, value any) bool {
		index.ratios.Delete(key)

		return true
	})
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
