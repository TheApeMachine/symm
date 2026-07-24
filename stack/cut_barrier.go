package stack

import (
	"sync"

	"github.com/theapemachine/symm/types"
)

/*
cutBarrier tracks per-cut signal results until every registered source reports.
*/
type cutBarrier struct {
	mu      sync.Mutex
	sources []types.SourceType
	pending map[types.CutID]map[types.SourceType]types.SignalResult
	active  types.CutID
}

/*
newCutBarrier constructs an empty barrier for the listed sources.
*/
func newCutBarrier(sources []types.SourceType) cutBarrier {
	return cutBarrier{
		sources: append([]types.SourceType(nil), sources...),
		pending: make(map[types.CutID]map[types.SourceType]types.SignalResult),
	}
}

func (barrier *cutBarrier) open(cutID types.CutID) {
	barrier.mu.Lock()
	defer barrier.mu.Unlock()

	barrier.active = cutID
	barrier.pending[cutID] = make(
		map[types.SourceType]types.SignalResult, len(barrier.sources),
	)
}

/*
Report records one signal result for the active or specified cut.
*/
func (barrier *cutBarrier) Report(result types.SignalResult) {
	barrier.mu.Lock()
	defer barrier.mu.Unlock()

	if result.CutID == 0 {
		result.CutID = barrier.active
	}

	if result.CutID == 0 {
		return
	}

	bucket, ok := barrier.pending[result.CutID]

	if !ok {
		bucket = make(map[types.SourceType]types.SignalResult, len(barrier.sources))
		barrier.pending[result.CutID] = bucket
	}

	bucket[result.Source] = result
}

func (barrier *cutBarrier) autoSkip(cutID types.CutID) {
	// ponytail: non-Hawkes sources are not yet wired to Report SignalResult;
	// treat absence as Skip so the barrier still closes on Hawkes cadence.
	// Remove this when per-source reporting is connected end-to-end.
	barrier.mu.Lock()
	defer barrier.mu.Unlock()

	bucket := barrier.pending[cutID]

	for _, source := range barrier.sources {
		if source == types.SourceHawkes {
			continue
		}

		if _, ok := bucket[source]; ok {
			continue
		}

		bucket[source] = types.SignalResult{
			CutID:  cutID,
			Source: source,
			Status: types.SignalSkip,
		}
	}
}

func (barrier *cutBarrier) fillMissing(cutID types.CutID) {
	barrier.autoSkip(cutID)
}

func (barrier *cutBarrier) ready(cutID types.CutID) bool {
	barrier.mu.Lock()
	defer barrier.mu.Unlock()

	bucket := barrier.pending[cutID]

	if len(bucket) < len(barrier.sources) {
		return false
	}

	for _, source := range barrier.sources {
		if _, ok := bucket[source]; !ok {
			return false
		}
	}

	return true
}

func (barrier *cutBarrier) clear(cutID types.CutID) {
	barrier.mu.Lock()
	defer barrier.mu.Unlock()

	delete(barrier.pending, cutID)
}
