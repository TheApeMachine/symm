package audit

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/types"
)

/*
PriceProvider abstracts the live tape so we can fetch the current price
for matured decisions without importing broker packages in audit.
*/
type PriceProvider interface {
	Mark(symbol string, direction string) float64
}

/*
ObserverFeedback defines the feedback sink to notify the regulator of outcomes.
*/
type ObserverFeedback interface {
	ObserveMark(feedback types.MarkFeedback) error
}

/*
ConcurrentObserver monitors matured decisions from the staging buffer
and evaluates them against the elapsed tape. Decisions representing missed
opportunities or executed trades are flushed to permanent storage; useless
decisions are pruned.
*/
type ConcurrentObserver struct {
	stager        *Stager
	priceProvider PriceProvider
	regulator     ObserverFeedback
	checkInterval time.Duration
	thresholdPct  float64
	mu            sync.Mutex
}

/*
NewConcurrentObserver creates a background evaluator for staged decisions.
*/
func NewConcurrentObserver(
	stager *Stager,
	provider PriceProvider,
	regulator ObserverFeedback,
) *ConcurrentObserver {
	return &ConcurrentObserver{
		stager:        stager,
		priceProvider: provider,
		regulator:     regulator,
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
	if co.stager == nil || co.priceProvider == nil {
		return
	}

	matured := co.stager.Matured()

	for _, decision := range matured {
		currentPrice := co.priceProvider.Mark(decision.Symbol, "sell")

		if currentPrice <= 0 || decision.Mark == nil {
			// No tape data to evaluate or no entry mark; assume useless and prune
			co.stager.Prune(decision.ID)
			continue
		}

		entryPrice := decision.Mark.Float64()

		if entryPrice > 0 {
			excursion := (currentPrice - entryPrice) / entryPrice

			// If we took an action, or if it was a missed opportunity, keep it
			if decision.Action != types.ActionNothing || math.Abs(excursion) >= co.thresholdPct {
				_ = co.stager.Flush(decision.ID)

				if co.regulator != nil {
					_ = co.regulator.ObserveMark(types.MarkFeedback{
						Symbol:        decision.Symbol,
						PositionID:    decision.ID,
						Mark:          currentPrice,
						At:            time.Now(),
						PeakDrawdown:  math.Min(0, excursion),
						FloorDistance: 0,
						Exposed:       decision.Action != types.ActionNothing,
					})
				}

				continue
			}
		}

		// Otherwise, it was uninteresting. Prune it.
		co.stager.Prune(decision.ID)
	}
}
