package hawkes

import (
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/nomagique/hawkes"
)

func (symbol *symbol) computeFit(
	context hawkes.FitContext,
	stream hawkes.ArrivalStream,
	horizon time.Time,
	prior hawkes.BivariateFit,
) (fitEpoch, bool) {
	estimator := hawkes.NewBivariateEstimator(prior)
	fit := estimator.Fit(stream, horizon)

	if !fit.Valid() {
		return fitEpoch{}, false
	}

	selfOnly := estimator.FitSelfOnly(stream, horizon)

	if !selfOnly.Valid() {
		return fitEpoch{}, false
	}

	fullLikelihood := fit.LogLikelihood(stream, horizon)
	selfLikelihood := selfOnly.LogLikelihood(stream, horizon)

	if selfLikelihood > fullLikelihood {
		fit = selfOnly
	}

	_, _, immediateReady := fit.ImmediateOffspring()
	_, _, totalReady := fit.TotalDescendants()

	if !immediateReady || !totalReady {
		return fitEpoch{}, false
	}

	return fitEpoch{
		model:        fit,
		restricted:   selfOnly,
		prior:        prior,
		observedFrom: context.ObservedFrom,
		at:           horizon,
		eventCount:   context.TotalEvents,
	}, true
}

func (symbol *symbol) publish(
	epoch *fitEpoch,
	context hawkes.FitContext,
	stream hawkes.ArrivalStream,
	horizon time.Time,
) Out {
	symbol.fitPrior = epoch.prior
	symbol.model = epoch.model
	symbol.restricted = epoch.restricted
	symbol.hasFit = true
	symbol.fitObservedFrom = epoch.observedFrom
	symbol.fitAt = epoch.at
	symbol.schedule.Reset(epoch.eventCount)

	outcome := symbol.evaluate(
		context,
		stream,
		horizon,
		epoch.model,
		epoch.restricted,
		true,
	)
	outcome.FitObservedFrom = symbol.fitObservedFrom
	outcome.FitObservedAt = symbol.fitAt

	return outcome
}

func (symbol *symbol) project(
	context hawkes.FitContext,
	stream hawkes.ArrivalStream,
	horizon time.Time,
) Out {
	outcome := symbol.evaluate(
		context,
		stream,
		horizon,
		symbol.model,
		symbol.restricted,
		false,
	)
	outcome.FitObservedFrom = symbol.fitObservedFrom
	outcome.FitObservedAt = symbol.fitAt
	outcome.Readiness.Reason = fmt.Sprintf(
		"conditional intensity uses retained fit; %d changed events remain before refit; %s",
		symbol.schedule.Remaining(),
		forecastPendingReason,
	)

	return outcome
}

func (symbol *symbol) evaluate(
	context hawkes.FitContext,
	stream hawkes.ArrivalStream,
	horizon time.Time,
	model hawkes.BivariateFit,
	restricted hawkes.BivariateFit,
	modelUpdated bool,
) Out {
	fit := model.WithIntensitiesAt(stream, horizon)
	fullLikelihood := fit.LogLikelihood(stream, horizon)
	selfLikelihood := restricted.WithIntensitiesAt(stream, horizon).
		LogLikelihood(stream, horizon)
	poisson := context.PoissonFit().WithIntensitiesAt(stream, horizon)
	immediateBuy, immediateSell, _ := model.ImmediateOffspring()
	totalBuy, totalSell, _ := model.TotalDescendants()
	outcome := symbol.outcome(context, stream, horizon, fit)
	outcome.HawkesPoissonLogLikelihoodDelta =
		fullLikelihood - poisson.LogLikelihood(stream, horizon)
	outcome.CrossSelfLogLikelihoodDelta =
		fullLikelihood - selfLikelihood
	outcome.ImmediateAlphaOffspring = immediateBuy
	outcome.ImmediateBetaOffspring = immediateSell
	outcome.TotalAlphaDescendants = totalBuy
	outcome.TotalBetaDescendants = totalSell
	outcome.Maturity = 1
	outcome.Readiness = Readiness{
		Observation:  true,
		Intensity:    true,
		HawkesFit:    true,
		ModelUpdated: modelUpdated,
		Reason:       forecastPendingReason,
	}

	return outcome
}

func (symbol *symbol) outcome(
	context hawkes.FitContext,
	stream hawkes.ArrivalStream,
	horizon time.Time,
	fit hawkes.BivariateFit,
) Out {
	observedFrom := context.ObservedFrom
	buyCount := context.EventsX
	sellCount := context.EventsY
	eventCount := buyCount + sellCount
	maturity := math.Min(
		float64(eventCount)/float64(context.MinFitEvents),
		math.Min(
			float64(buyCount)/float64(context.MinPerSide),
			float64(sellCount)/float64(context.MinPerSide),
		),
	)
	span := horizon.Sub(observedFrom)
	buyRate := float64(buyCount) / span.Seconds()
	sellRate := float64(sellCount) / span.Seconds()

	if maturity > 1 {
		maturity = 1
	}

	return Out{
		Fit:              fit,
		ObservedFrom:     observedFrom,
		ObservedAt:       horizon,
		Horizon:          horizon.Sub(observedFrom),
		EventCount:       eventCount,
		AlphaEventCount:  buyCount,
		BetaEventCount:   sellCount,
		AlphaArrivalRate: buyRate,
		BetaArrivalRate:  sellRate,
		MinimumFitEvents: context.MinFitEvents,
		Maturity:         maturity,
	}
}

func (symbol *symbol) context(
	stream hawkes.ArrivalStream,
	horizon time.Time,
) (hawkes.FitContext, hawkes.ArrivalStream, bool) {
	context, ready := hawkes.NewObservationContext(stream, horizon)

	if !ready {
		return hawkes.FitContext{}, hawkes.ArrivalStream{}, false
	}

	origin := horizon.Add(-context.TradeWindow)

	if origin.Before(stream.ObservationOrigin()) {
		origin = stream.ObservationOrigin()
	}

	observed := stream.WithObservationOrigin(origin)

	context, ready = hawkes.NewObservationContext(observed, horizon)

	return context, observed, ready
}
