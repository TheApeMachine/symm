package hawkes

import (
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/numeric/adaptive"
)

type HawkesSymbol struct {
	fit             BivariateFit
	hasFit          bool
	lastFitEventKey fitEventKey
	lastFitTime     time.Time
	fitCooldown     time.Duration
	minFitEvents    int
	rawBase         *adaptive.EMA
	lastCategory    logic.CategoryType
}

type hawkesReading struct {
	category   logic.CategoryType
	strength   float64
	confidence float64
	frenzy     float64
	saturation float64
	organic    float64
	exhaustion float64
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

func (sym *HawkesSymbol) Measure(
	ticks []market.TradeUpdate, now time.Time,
) (hawkesReading, bool) {
	context, stream, ok := FitContextFromTicks(ticks, time.Time{}, now)

	if !ok || !context.EnoughEvents(stream) {
		if !sym.hasFit {
			return hawkesReading{}, false
		}

		stream = ArrivalStreamFromTicks(ticks, time.Time{}, now)

		if len(stream.Marked()) == 0 {
			return hawkesReading{}, false
		}

		return sym.measureFit(sym.fit.WithIntensitiesAt(stream, now))
	}

	fit, ok := sym.fitForEvents(stream, now)

	if !ok {
		return hawkesReading{}, false
	}

	return sym.measureFit(fit)
}

func (sym *HawkesSymbol) measureFit(fit BivariateFit) (hawkesReading, bool) {
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

	category, confidence, frenzy, saturation, organic, exhaustion := classifyHawkes(
		fit, asymmetry, sellSide,
	)

	rawNorm := sym.rawBase.Value()
	_, _ = sym.rawBase.Next(0, raw)

	if rawNorm > 0 {
		saturationEvidence := competitionMargin(raw-rawNorm, rawNorm) * (1 - asymmetry)

		if saturationEvidence > confidence {
			category = logic.CategorySaturation
			confidence = saturationEvidence
			saturation = saturationEvidence
		}
	}

	if sym.lastCategory == logic.CategoryFrenzy ||
		sym.lastCategory == logic.CategorySaturation {
		exhaustionEvidence := competitionMargin(rawNorm-raw, rawNorm)

		if exhaustionEvidence > confidence {
			category = logic.CategoryExhaustion
			confidence = exhaustionEvidence
			exhaustion = exhaustionEvidence
		}
	}

	sym.lastCategory = category

	return hawkesReading{
		category:   category,
		strength:   raw,
		confidence: confidence,
		frenzy:     frenzy,
		saturation: saturation,
		organic:    organic,
		exhaustion: exhaustion,
	}, true
}
