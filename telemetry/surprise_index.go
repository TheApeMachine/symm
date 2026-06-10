package telemetry

import (
	"math"
	"sort"
	"sync"
)

/*
SurpriseIndex aggregates per-source surprise/threshold ratios into one market
chaos gauge that modulates adaptive baseline forgetting rates.
*/
type SurpriseIndex struct {
	mu     sync.RWMutex
	ratios map[string]float64
}

func NewSurpriseIndex() *SurpriseIndex {
	return &SurpriseIndex{
		ratios: make(map[string]float64),
	}
}

func (index *SurpriseIndex) Record(source string, surprise, threshold float64) {
	if source == "" || surprise <= 0 || threshold <= 0 {
		return
	}

	index.mu.Lock()
	index.ratios[source] = surprise / threshold
	index.mu.Unlock()
}

func (index *SurpriseIndex) Index() float64 {
	index.mu.RLock()
	defer index.mu.RUnlock()

	if len(index.ratios) == 0 {
		return 1
	}

	values := make([]float64, 0, len(index.ratios))

	for _, ratio := range index.ratios {
		values = append(values, ratio)
	}

	sort.Float64s(values)

	middle := len(values) / 2

	if len(values)%2 == 1 {
		return values[middle]
	}

	return (values[middle-1] + values[middle]) / 2
}

func (index *SurpriseIndex) Reset() {
	index.mu.Lock()
	clear(index.ratios)
	index.mu.Unlock()
}

func (index *SurpriseIndex) SnapshotRatios() map[string]float64 {
	index.mu.RLock()
	defer index.mu.RUnlock()

	snapshot := make(map[string]float64, len(index.ratios))

	for source, ratio := range index.ratios {
		snapshot[source] = ratio
	}

	return snapshot
}

func (index *SurpriseIndex) RestoreRatios(ratios map[string]float64) {
	index.mu.Lock()
	defer index.mu.Unlock()

	index.ratios = make(map[string]float64, len(ratios))

	for source, ratio := range ratios {
		index.ratios[source] = ratio
	}
}

var sharedSurpriseIndex = NewSurpriseIndex()

func SharedSurpriseIndex() *SurpriseIndex {
	return sharedSurpriseIndex
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
