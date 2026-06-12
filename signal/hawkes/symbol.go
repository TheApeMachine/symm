package hawkes

import (
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique"
	nomadaptive "github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	hkernel "github.com/theapemachine/nomagique/kernel/hawkes"
)

type HawkesSymbol struct {
	fit             hkernel.BivariateFit
	hasFit          bool
	lastFitEventKey fitEventKey
	lastFitTime     time.Time
	fitCooldown     time.Duration
	minFitEvents    int
	rawBase         *nomadaptive.Exponential
	lastRawNorm     float64
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
		rawBase:      nomadaptive.EMA(),
	}
}

func (sym *HawkesSymbol) FitBivariate(
	stream hkernel.ArrivalStream,
	horizon time.Time,
) hkernel.BivariateFit {
	prior := hkernel.BivariateFit{}

	if sym.hasFit {
		prior = sym.fit
	}

	if context, ok := hkernel.NewFitContext(stream, horizon); ok {
		sym.minFitEvents = context.MinFitEvents
	}

	fit := hkernel.NewBivariateEstimator(prior).Fit(stream, horizon)

	if fit.MuX > 0 {
		sym.fit = fit
		sym.hasFit = true
	}

	return fit
}

func (sym *HawkesSymbol) fitForEvents(
	stream hkernel.ArrivalStream,
	horizon time.Time,
) (hkernel.BivariateFit, bool) {
	key := revisionKey(stream)

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

	if fit.MuX <= 0 {
		return hkernel.BivariateFit{}, false
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

		if len(stream.BuyTimes())+len(stream.SellTimes()) == 0 {
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

func (sym *HawkesSymbol) measureFit(fit hkernel.BivariateFit) (hawkesReading, bool) {
	sellSide := fit.Asymmetry(true) > fit.Asymmetry(false)
	asymmetry := fit.Asymmetry(sellSide)

	intensity, mu := fit.IntensityX, fit.MuX

	if sellSide {
		intensity, mu = fit.IntensityY, fit.MuY
	}

	raw := 1.0

	if mu > 0 {
		raw = intensity / mu
	}

	category, confidence, frenzy, saturation, organic, exhaustion := classifyHawkes(
		fit, asymmetry, sellSide,
	)

	rawNorm := sym.lastRawNorm
	sym.lastRawNorm = float64(nomagique.Scalar(raw).Observe(sym.rawBase))

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
