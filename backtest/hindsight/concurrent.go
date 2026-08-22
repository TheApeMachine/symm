package hindsight

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/audit"
)

/*
ConcurrentObserver monitors matured decisions from the staging buffer
and evaluates them against the elapsed tape. Decisions representing missed
opportunities or executed trades are flushed to permanent storage; useless
decisions are pruned.
*/
type ConcurrentObserver struct {
	stager        *audit.Stager
	reducer       *Reducer
	checkInterval time.Duration
	thresholdPct  float64
	mu            sync.Mutex
}

/*
NewConcurrentObserver creates a background evaluator for staged decisions.
*/
func NewConcurrentObserver(stager *audit.Stager, reducer *Reducer) *ConcurrentObserver {
	return &ConcurrentObserver{
		stager:        stager,
		reducer:       reducer,
		checkInterval: 10 * time.Second,
		thresholdPct:  0.01, // 1% excursion marks a missed opportunity
	}
}

/*
Run starts the background evaluation loop until the context is canceled.
*/
func (co *ConcurrentObserver) Run(ctx context.Context) {
	ticker := time.NewTicker(co.checkInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			co.evaluateMatured()
		}
	}
}

func (co *ConcurrentObserver) evaluateMatured() {
	if co.stager == nil || co.reducer == nil {
		return
	}
	
	matured := co.stager.Matured()
	for _, decision := range matured {
		series := co.reducer.SeriesFor(decision.Symbol)
		if series == nil || len(series.Points) == 0 {
			// No tape data to evaluate; assume useless and prune
			co.stager.Prune(decision.ID)
			continue
		}
		
		// Find the price at decision time
		var entryPrice float64
		var maxPrice float64
		
		for _, point := range series.Points {
			if !point.At.Before(decision.At) && entryPrice == 0 {
				entryPrice = point.Price
				maxPrice = point.Price
			}
			if entryPrice > 0 {
				if point.Price > maxPrice {
					maxPrice = point.Price
				}
			}
		}
		
		if entryPrice > 0 {
			excursion := (maxPrice - entryPrice) / entryPrice
			
			// If we took an action, or if it was a missed opportunity, keep it
			if decision.Action != "nothing" || math.Abs(excursion) >= co.thresholdPct {
				_ = co.stager.Flush(decision.ID)
				continue
			}
		}
		
		// Otherwise, it was uninteresting. Prune it.
		co.stager.Prune(decision.ID)
	}
}
