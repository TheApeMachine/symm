package hawkes

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/nomagique/hawkes"
)

const forecastPendingReason = "forecast readiness requires residual and out-of-sample validation"

/*
symbol owns the adaptive observation workspace, fitted parameter epoch, and
evidence-derived invalidation schedule for one market. Ingestion evaluates with
the latest retained parameters; unrestricted and restricted fits run on an
immutable snapshot and publish a new epoch atomically.
*/
type symbol struct {
	model           hawkes.BivariateFit
	restricted      hawkes.BivariateFit
	fitPrior        hawkes.BivariateFit
	hasFit          bool
	fitObservedFrom time.Time
	fitAt           time.Time
	lastOutcome     Out
	lastReady       bool
	adaptive        *hawkes.ArrivalWorkspace
	revision        *revision
	schedule        *schedule
	mu              sync.Mutex
	fitting         atomic.Bool
	pending         *fitEpoch
	fitSignal       chan struct{}
}

type fitEpoch struct {
	model        hawkes.BivariateFit
	restricted   hawkes.BivariateFit
	prior        hawkes.BivariateFit
	observedFrom time.Time
	at           time.Time
	eventCount   int
}

func newSymbol() *symbol {
	return &symbol{
		adaptive:  hawkes.NewArrivalWorkspace(),
		revision:  &revision{},
		schedule:  &schedule{},
		fitSignal: make(chan struct{}, 1),
	}
}

func (symbol *symbol) measure(
	stream hawkes.ArrivalStream,
	horizon time.Time,
) (Out, bool) {
	context, observed, ready := symbol.context(stream, horizon)

	if !ready {
		return symbol.retain(symbol.observation(stream, horizon)), true
	}

	changedEvents := symbol.revision.Observe(observed)

	if symbol.hasFit {
		symbol.schedule.Observe(changedEvents)
	}

	if epoch := symbol.takePending(); epoch != nil {
		return symbol.retain(symbol.publish(epoch, context, observed, horizon)), true
	}

	if !context.EnoughEvents(observed) {
		return symbol.retain(symbol.baseline(context, observed, horizon)), true
	}

	if symbol.hasFit {
		outcome := symbol.project(context, observed, horizon)

		if symbol.schedule.Ready() {
			symbol.requestFit(context, observed, horizon)
		}

		return symbol.retain(outcome), true
	}

	epoch, ok := symbol.computeFit(context, observed, horizon, hawkes.BivariateFit{})

	if !ok {
		return Out{}, false
	}

	return symbol.retain(symbol.publish(&epoch, context, observed, horizon)), true
}

func (symbol *symbol) retain(outcome Out) Out {
	symbol.lastOutcome = outcome
	symbol.lastReady = true

	return outcome
}

func (symbol *symbol) observation(
	stream hawkes.ArrivalStream,
	horizon time.Time,
) Out {
	observedFrom := stream.ObservationOrigin()
	context, ready := hawkes.NewObservationContext(stream, horizon)

	if ready {
		return symbol.outcome(context, stream, horizon, hawkes.BivariateFit{})
	}

	buyCount, sellCount := stream.ObservationCounts(horizon)

	if horizon.Equal(observedFrom) {
		for _, eventTime := range stream.BuyTimes() {
			if eventTime.Equal(horizon) {
				buyCount++
			}
		}

		for _, eventTime := range stream.SellTimes() {
			if eventTime.Equal(horizon) {
				sellCount++
			}
		}
	}

	return Out{
		ObservedFrom:    observedFrom,
		ObservedAt:      horizon,
		Horizon:         horizon.Sub(observedFrom),
		EventCount:      buyCount + sellCount,
		AlphaEventCount: buyCount,
		BetaEventCount:  sellCount,
		Readiness: Readiness{
			Observation: true,
			Reason:      "arrival rate requires a positive observation interval",
		},
	}
}

func (symbol *symbol) baseline(
	context hawkes.FitContext,
	stream hawkes.ArrivalStream,
	horizon time.Time,
) Out {
	outcome := symbol.outcome(
		context,
		stream,
		horizon,
		context.PoissonFit(),
	)
	outcome.Readiness = Readiness{
		Observation: true,
		Intensity:   true,
		Reason:      symbol.pendingReason(context, stream),
	}

	return outcome
}

func (symbol *symbol) pendingReason(
	context hawkes.FitContext,
	stream hawkes.ArrivalStream,
) string {
	buyCount := context.EventsX
	sellCount := context.EventsY

	if buyCount < context.MinPerSide || sellCount < context.MinPerSide {
		return fmt.Sprintf(
			"hawkes fit requires %d events per side; observed buy=%d sell=%d",
			context.MinPerSide,
			buyCount,
			sellCount,
		)
	}

	return fmt.Sprintf(
		"hawkes fit requires %d events; observed %d",
		context.MinFitEvents,
		buyCount+sellCount,
	)
}

func (symbol *symbol) requestFit(
	context hawkes.FitContext,
	stream hawkes.ArrivalStream,
	horizon time.Time,
) {
	if !symbol.fitting.CompareAndSwap(false, true) {
		return
	}

	prior := symbol.model
	origin := stream.ObservationOrigin()
	buyTimes := append([]time.Time(nil), stream.BuyTimes()...)
	sellTimes := append([]time.Time(nil), stream.SellTimes()...)

	go func() {
		defer symbol.fitting.Store(false)

		snapshot := hawkes.NewArrivalStreamFrom(origin, buyTimes, sellTimes)
		epoch, ok := symbol.computeFit(context, snapshot, horizon, prior)

		if !ok {
			symbol.notifyFit()

			return
		}

		symbol.mu.Lock()
		symbol.pending = &epoch
		symbol.mu.Unlock()
		symbol.notifyFit()
	}()
}

func (symbol *symbol) takePending() *fitEpoch {
	symbol.mu.Lock()
	defer symbol.mu.Unlock()

	pending := symbol.pending
	symbol.pending = nil

	return pending
}

func (symbol *symbol) notifyFit() {
	select {
	case symbol.fitSignal <- struct{}{}:
	default:
	}
}

func (symbol *symbol) awaitFit() bool {
	if !symbol.fitting.Load() && symbol.peekPending() {
		return true
	}

	if !symbol.fitting.Load() {
		return symbol.peekPending()
	}

	<-symbol.fitSignal

	return symbol.peekPending() || !symbol.fitting.Load()
}

func (symbol *symbol) peekPending() bool {
	symbol.mu.Lock()
	defer symbol.mu.Unlock()

	return symbol.pending != nil
}
