package hawkes

import (
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
HawkesSymbol fits a bivariate self-exciting Hawkes process to one symbol's
buy/sell trade arrivals and classifies the excitation state onto the thermal
perspective. The fit is cooldown-throttled and refreshed in place between
refits — a full MLE per tick would saturate a core.

SNR is the dominant side's current intensity over its own exogenous baseline μ:
a feedback loop running hot above background clears the noise floor.
*/
type HawkesSymbol struct {
	fit             BivariateFit
	hasFit          bool
	lastFitEventKey fitEventKey
	lastFitTime     time.Time
	fitCooldown     time.Duration
	minFitEvents    int
}

func NewHawkesSymbol() *HawkesSymbol {
	return &HawkesSymbol{
		minFitEvents: bivariateParamCount * 2,
		fitCooldown:  config.System.HawkesFitCooldown,
	}
}

func (sym *HawkesSymbol) FitBivariate(stream ArrivalStream, horizon time.Time) BivariateFit {
	prior := BivariateFit{}

	if sym.hasFit {
		prior = sym.fit
	}

	if context, ok := NewFitContext(stream, horizon); ok {
		sym.minFitEvents = context.MinFitEvents
	}

	fit := NewBivariateEstimator(prior).Fit(stream, horizon)

	if fit.MuBuy > 0 {
		sym.fit = fit
		sym.hasFit = true
	}

	return fit
}

func (sym *HawkesSymbol) fitForEvents(stream ArrivalStream, horizon time.Time) (BivariateFit, bool) {
	key := stream.RevisionKey()

	if sym.hasFit && key == sym.lastFitEventKey {
		return sym.fit.WithIntensitiesAt(stream, horizon), true
	}

	if sym.hasFit &&
		sym.fitCooldown > 0 &&
		!sym.lastFitTime.IsZero() &&
		horizon.Sub(sym.lastFitTime) < sym.fitCooldown {
		return sym.fit.WithIntensitiesAt(stream, horizon), true
	}

	fit := sym.FitBivariate(stream, horizon)

	if fit.MuBuy <= 0 {
		return BivariateFit{}, false
	}

	sym.lastFitEventKey = key
	sym.lastFitTime = horizon

	return fit, true
}

/*
Measure fits the arrival stream and emits the thermal reading. SNR is the
dominant-side intensity relative to its exogenous baseline μ.
*/
func (sym *HawkesSymbol) Measure(ticks []market.TradeUpdate, now time.Time) (perspectives.Measurement, bool) {
	context, stream, ok := FitContextFromTicks(ticks, time.Time{}, now)

	if !ok || !context.EnoughEvents(stream) {
		return perspectives.Measurement{}, false
	}

	fit, ok := sym.fitForEvents(stream, now)

	if !ok {
		return perspectives.Measurement{}, false
	}

	sellSide := fit.Asymmetry(true) > fit.Asymmetry(false)
	asymmetry := fit.Asymmetry(sellSide)

	intensity, mu := fit.BuyIntensity, fit.MuBuy

	if sellSide {
		intensity, mu = fit.SellIntensity, fit.MuSell
	}

	raw := 1.0

	if mu > 0 {
		raw = intensity / mu
	}

	return perspectives.WithGaugeFactors(perspectives.Measurement{
		Source:   perspectives.SourceHawkes,
		Category: hawkesCategory(fit, asymmetry, sellSide),
		Strength: raw,
	}, []perspectives.GaugeFactor{
		{Name: "mu", Value: mu},
		{Name: "intensity", Value: intensity},
		{Name: "asymmetry", Value: asymmetry},
	}), true
}

/*
hawkesCategory maps the fitted Hawkes state onto the thermal perspective: a
spectral radius approaching the critical branch is contested saturation; a
dominant-side intensity below its own exogenous baseline is exhaustion; a
strongly one-sided cluster is a directional frenzy; otherwise organic.
*/
func hawkesCategory(fit BivariateFit, asymmetry float64, sellSide bool) perspectives.CategoryType {
	intensity, mu := fit.BuyIntensity, fit.MuBuy

	if sellSide {
		intensity, mu = fit.SellIntensity, fit.MuSell
	}

	switch {
	case fit.SpectralRadius >= 0.85:
		return perspectives.CategorySaturation
	case mu > 0 && intensity < mu:
		return perspectives.CategoryExhaustion
	case asymmetry >= 0.15:
		return perspectives.CategoryFrenzy
	default:
		return perspectives.CategoryOrganic
	}
}
