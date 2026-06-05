package hawkes

import (
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
)

/*
HawkesSymbol fits a bivariate self-exciting Hawkes process to one symbol's
buy/sell trade arrivals and classifies the excitation state onto the thermal
perspective. The fit is cooldown-throttled and refreshed in place between
refits — a full MLE per tick would saturate a core.

Confidence is classification clarity — margin to the saturation, exhaustion, or
frenzy boundary; SNR is how surprising that clarity is versus the symbol's own
recent baseline, not intensity-over-μ.
*/
type HawkesSymbol struct {
	fit             BivariateFit
	hasFit          bool
	lastFitEventKey fitEventKey
	lastFitTime     time.Time
	fitCooldown     time.Duration
	minFitEvents    int
	rawBase         *adaptive.EMA
	tracked         *perspectives.Category
}

func hawkesFitCooldown() time.Duration {
	if raw := viper.GetString("signals.hawkes_fit_cooldown"); raw != "" {
		if duration, err := time.ParseDuration(raw); err == nil {
			return duration
		}
	}

	if seconds := viper.GetInt("signals.hawkes_fit_cooldown"); seconds > 0 {
		return time.Duration(seconds) * time.Second
	}

	return viper.GetDuration("signals.hawkes_fit_cooldown")
}

func NewHawkesSymbol() *HawkesSymbol {
	return &HawkesSymbol{
		minFitEvents: bivariateParamCount * 2,
		fitCooldown:  hawkesFitCooldown(),
		rawBase:      adaptive.NewEMA(0),
		tracked:      perspectives.NewCategory(perspectives.CategoryTypeNone),
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
Measure fits the arrival stream and emits the thermal reading. Confidence is
category clarity — how decisively the fitted state lands in its assigned category.
*/
func (sym *HawkesSymbol) Measure(
	ticks []market.TradeUpdate, now time.Time,
) (perspectives.Measurement, float64, error) {
	context, stream, ok := FitContextFromTicks(ticks, time.Time{}, now)

	if !ok || !context.EnoughEvents(stream) {
		if !sym.hasFit {
			return perspectives.Measurement{}, 0, nil
		}

		stream = ArrivalStreamFromTicks(ticks, time.Time{}, now)

		if len(stream.Marked()) == 0 {
			return perspectives.Measurement{}, 0, nil
		}

		return sym.measureFit(sym.fit.WithIntensitiesAt(stream, now))
	}

	fit, ok := sym.fitForEvents(stream, now)

	if !ok {
		return perspectives.Measurement{}, 0, nil
	}

	return sym.measureFit(fit)
}

func (sym *HawkesSymbol) measureFit(fit BivariateFit) (perspectives.Measurement, float64, error) {
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

	// clarity is how decisively the fitted state lands in its category (the
	// boundary margin); standout is the strength of the self-exciting process
	// itself — the intensity ratio above baseline — which SNR scores against this
	// symbol's own history. They are different questions, so they are different
	// numbers: a weak excitation can still land cleanly in a category, and a violent
	// one can sit right on a boundary.
	category, evidence := hawkesReading(fit, asymmetry, sellSide)
	rawNorm := sym.rawBase.Value()
	_, _ = sym.rawBase.Next(0, raw)

	if rawNorm > 0 {
		saturationEvidence := perspectives.UnitCompetitionMargin(raw-rawNorm, rawNorm) *
			(1 - asymmetry)

		if saturationEvidence > evidence {
			category = perspectives.CategorySaturation
			evidence = saturationEvidence
		}
	}

	if sym.tracked.Type == perspectives.CategoryFrenzy ||
		sym.tracked.Type == perspectives.CategorySaturation {
		exhaustionEvidence := perspectives.UnitCompetitionMargin(rawNorm-raw, rawNorm)

		if exhaustionEvidence > 0 {
			category = perspectives.CategoryExhaustion
			evidence = exhaustionEvidence
		}
	}

	clarity := evidence
	standout := perspectives.UnitMagnitudeMargin(raw)

	confidence, err := sym.tracked.Observe(category, clarity, standout)

	if err != nil {
		return perspectives.Measurement{}, 0, err
	}

	return perspectives.Measurement{
		Source:     perspectives.SourceHawkes,
		Category:   category,
		Strength:   raw,
		Confidence: confidence,
	}, standout, nil
}
