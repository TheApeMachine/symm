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
	ObserveHindsight(feedback types.HindsightFeedback) error
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
		thresholdPct:  0.01,
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
			co.stager.Prune(decision.ID)
			continue
		}

		entryPrice := decision.Mark.Float64()

		if entryPrice <= 0 {
			co.stager.Prune(decision.ID)
			continue
		}

		excursion := (currentPrice - entryPrice) / entryPrice
		isAction := decision.Action != types.ActionNothing
		isMissed := !isAction && excursion >= co.thresholdPct

		if isAction || isMissed {
			_ = co.stager.Flush(decision.ID)

			if co.regulator != nil {
				_ = co.regulator.ObserveMark(types.MarkFeedback{
					Symbol:        decision.Symbol,
					PositionID:    decision.ID,
					Mark:          currentPrice,
					At:            time.Now(),
					PeakDrawdown:  math.Min(0, excursion),
					FloorDistance: 0,
					Exposed:       isAction,
				})

				_ = co.regulator.ObserveHindsight(co.hindsightAttribution(
					decision,
					excursion,
					isAction,
					isMissed,
				))
			}

			continue
		}

		co.stager.Prune(decision.ID)
	}
}

func (co *ConcurrentObserver) hindsightAttribution(
	decision *types.Decision,
	excursion float64,
	isAction bool,
	isMissed bool,
) types.HindsightFeedback {
	dominantBlocker := "unspecified"
	minMargin := 0.0

	thesisMargin := decision.Alternatives["admission:thesis_score_margin"]
	confMargin := decision.Alternatives["admission:confidence_margin"]
	suppMargin := decision.Alternatives["admission:support_margin"]
	contMargin := decision.Alternatives["admission:contradiction_margin"]
	graphMargin := decision.GraphScore - decision.AdmissionGraphThreshold

	if isMissed {
		if thesisMargin < minMargin {
			minMargin = thesisMargin
			dominantBlocker = "thesis_score"
		}

		if confMargin < minMargin {
			minMargin = confMargin
			dominantBlocker = "confidence"
		}

		if suppMargin < minMargin {
			minMargin = suppMargin
			dominantBlocker = "support"
		}

		if contMargin < minMargin {
			minMargin = contMargin
			dominantBlocker = "contradiction"
		}

		if graphMargin < minMargin {
			minMargin = graphMargin
			dominantBlocker = "graph"
		}
	}

	missedReturn := 0.0

	if isMissed {
		missedReturn = excursion
	}

	return types.HindsightFeedback{
		At:                  time.Now(),
		Symbol:              decision.Symbol,
		Opportunity:         decision.Opportunity || math.Abs(excursion) >= co.thresholdPct,
		OpportunityType:     decision.OpportunityType,
		Captured:            isAction,
		Missed:              isMissed,
		RealizedReturn:      excursion,
		MissedReturn:        missedReturn,
		HoldingDuration:     time.Since(decision.At),
		DominantBlocker:     dominantBlocker,
		ThesisMargin:        thesisMargin,
		ConfidenceMargin:    confMargin,
		SupportMargin:       suppMargin,
		ContradictionMargin: contMargin,
		GraphMargin:         graphMargin,
	}
}
