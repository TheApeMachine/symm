package hawkes

import (
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
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
		return perspectives.Measurement{}, 0, nil
	}

	fit, ok := sym.fitForEvents(stream, now)

	if !ok {
		return perspectives.Measurement{}, 0, nil
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

	category, evidence := hawkesReading(fit, asymmetry, sellSide)
	standout := evidence
	clarity := evidence

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
