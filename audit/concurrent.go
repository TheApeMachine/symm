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
type observedMark struct {
	at    time.Time
	price float64
}

type ConcurrentObserver struct {
	stager        *Stager
	priceProvider PriceProvider
	regulator     ObserverFeedback
	checkInterval time.Duration
	thresholdPct  float64
	mu            sync.Mutex
	priceHistory  map[string][]observedMark
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
		checkInterval: time.Second,
		thresholdPct:  0.01,
		priceHistory:  make(map[string][]observedMark),
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
			co.samplePending()
			co.evaluateMatured()
		}
	}
}

func (co *ConcurrentObserver) samplePending() {
	if co.stager == nil || co.priceProvider == nil {
		return
	}

	now := time.Now().UTC()
	pending := co.stager.Pending()
	earliest := make(map[string]time.Time)

	for _, staged := range pending {
		decision := staged.Decision

		if decision == nil || decision.Symbol == "" {
			continue
		}

		start := decision.At
		if start.IsZero() {
			start = now
		}

		if known, ok := earliest[decision.Symbol]; !ok || start.Before(known) {
			earliest[decision.Symbol] = start
		}
	}

	co.mu.Lock()
	defer co.mu.Unlock()

	for symbol, start := range earliest {
		price := co.priceProvider.Mark(symbol, "sell")

		if price > 0 {
			co.priceHistory[symbol] = append(
				co.priceHistory[symbol],
				observedMark{at: now, price: price},
			)
		}

		history := co.priceHistory[symbol]
		first := 0

		for first < len(history) && history[first].at.Before(start) {
			first++
		}

		if first > 0 {
			history = append([]observedMark(nil), history[first:]...)
		}

		co.priceHistory[symbol] = history
	}

	for symbol := range co.priceHistory {
		if _, active := earliest[symbol]; !active {
			delete(co.priceHistory, symbol)
		}
	}
}

func (co *ConcurrentObserver) maximumPrice(symbol string, since time.Time, fallback float64) float64 {
	maximum := fallback

	co.mu.Lock()
	defer co.mu.Unlock()

	for _, sample := range co.priceHistory[symbol] {
		if sample.at.Before(since) {
			continue
		}

		if sample.price > maximum {
			maximum = sample.price
		}
	}

	return maximum
}

func (co *ConcurrentObserver) evaluateMatured() {
	if co.stager == nil || co.priceProvider == nil {
		return
	}

	matured := co.stager.Matured()

	for _, decision := range matured {
		if decision == nil {
			continue
		}

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

		terminalExcursion := (currentPrice - entryPrice) / entryPrice
		peakPrice := co.maximumPrice(decision.Symbol, decision.At, currentPrice)
		peakExcursion := (peakPrice - entryPrice) / entryPrice
		isAction := decision.Action != types.ActionNothing
		isMissed := !isAction && peakExcursion >= co.thresholdPct

		if isAction || isMissed {
			_ = co.stager.Flush(decision.ID)

			if co.regulator != nil {
				_ = co.regulator.ObserveMark(types.MarkFeedback{
					Symbol:        decision.Symbol,
					PositionID:    decision.ID,
					Mark:          currentPrice,
					At:            time.Now(),
					PeakDrawdown:  math.Min(0, terminalExcursion),
					FloorDistance: 0,
					Exposed:       isAction,
				})

				_ = co.regulator.ObserveHindsight(co.hindsightAttribution(
					decision,
					peakExcursion,
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
