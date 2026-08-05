package tests

import (
	"slices"
	"sync"

	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

/*
thesisCapture records the analyzed Thesis immediately before the real planner
consumes and may close its decision cycle. It is a test observation point, not
an alternate evaluator: Update always delegates to the production planner.
*/
type thesisCapture struct {
	mu      sync.RWMutex
	planner *strategy.Planner
	symbol  string
	latest  *types.Thesis
}

func newThesisCapture(planner *strategy.Planner) *thesisCapture {
	return &thesisCapture{planner: planner}
}

func (capture *thesisCapture) Enable(symbol string) {
	capture.mu.Lock()
	capture.symbol = symbol
	capture.latest = nil
	capture.mu.Unlock()
}

func (capture *thesisCapture) Update(thesis *types.Thesis) *types.Thesis {
	capture.mu.Lock()

	if capture.symbol != "" {
		if _, found := thesis.LatestTicker(capture.symbol); found {
			capture.latest = cloneThesisEvidence(thesis)
		}
	}

	capture.mu.Unlock()

	return capture.planner.Update(thesis)
}

func (capture *thesisCapture) Snapshot() *types.Thesis {
	capture.mu.RLock()
	defer capture.mu.RUnlock()

	return capture.latest
}

/*
cloneThesisEvidence detaches the canonical observations and analysis products
that a completed planner pass is allowed to clear. Measurement rows and
resonance values are immutable after publication, so their containers alone
need cloning.
*/
func cloneThesisEvidence(thesis *types.Thesis) *types.Thesis {
	snapshot := types.NewThesis(nil)
	snapshot.Status = thesis.Status
	snapshot.Tick = thesis.Tick
	snapshot.At = thesis.At

	for _, ticker := range thesis.MarketTickers() {
		snapshot.AppendTicker(ticker)
	}

	for _, trade := range thesis.MarketTrades() {
		snapshot.AppendTrade(trade)
	}

	thesis.Measurements.Range(func(key, value any) bool {
		rows, ok := value.([]*types.Measurement)

		if ok {
			snapshot.Measurements.Store(key, slices.Clone(rows))
		}

		return true
	})

	for symbol, categories := range thesis.Categories {
		snapshot.Categories[symbol] = slices.Clone(categories)
	}

	thesis.Resonance.Range(func(key, value any) bool {
		snapshot.Resonance.Store(key, value)

		return true
	})

	return snapshot
}
