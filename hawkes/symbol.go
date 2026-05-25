package hawkes

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/numeric/logic"
)

type HawkesSymbol struct {
	confidenceHistory []float64
	intensityRatios   []float64
	dailyQuoteVol     float64
	fit               BivariateFit
	hasFit            bool
	minFitEvents      int
	calibrator        engine.PredictionCalibrator
	liveScore         float64
	gauge             *numeric.Derived
}

func NewHawkesSymbol(calibrationParams engine.CalibrationParams) *HawkesSymbol {
	return &HawkesSymbol{
		confidenceHistory: make([]float64, 0, confidenceHistoryCap(bivariateParamCount*2)),
		intensityRatios:   make([]float64, 0, confidenceHistoryCap(bivariateParamCount*2)),
		minFitEvents:      bivariateParamCount * 2,
		calibrator:        engine.NewPredictionCalibrator(calibrationParams),
		gauge: numeric.NewDerived(
			numeric.WithDynamics(adaptive.NewProduct()),
		),
	}
}

func (sym *HawkesSymbol) FeedTicker(last, volumeBase float64) {
	sym.dailyQuoteVol = volumeBase * last
}

func (sym *HawkesSymbol) ApplyFeedback(feedback engine.PredictionFeedback) {
	sym.calibrator.Apply(feedback)
}

func (sym *HawkesSymbol) FitBivariate(
	buyEvents, sellEvents []time.Time,
	horizon time.Time,
) BivariateFit {
	prior := BivariateFit{}

	if sym.hasFit {
		prior = applyExcitationCalibration(sym.fit, sym.calibrator.Scale())
	}

	context, ok := newFitContext(buyEvents, sellEvents, horizon)

	if ok {
		sym.minFitEvents = context.MinFitEvents
	}

	fit := fitBivariateWithPrior(buyEvents, sellEvents, horizon, prior)

	if fit.MuBuy > 0 {
		sym.fit = fit
		sym.hasFit = true
		sym.recordIntensityRatio(fit.BuyIntensity / fit.MuBuy)
	}

	return fit
}

func (sym *HawkesSymbol) baselineIntensityFence() float64 {
	if len(sym.intensityRatios) == 0 {
		return 1
	}

	fence := engine.ConfidenceFence(sym.intensityRatios)

	if fence <= 0 {
		return 1
	}

	return fence
}

func (sym *HawkesSymbol) gaugeScore(rawScore float64, persist bool) float64 {
	if rawScore <= 0 {
		return 0
	}

	normalized := sym.calibrator.NormalizeConfidence(rawScore, sym.confidenceHistory)
	sym.liveScore = normalized

	if persist {
		sym.recordConfidence(rawScore)
	}

	return normalized
}

func (sym *HawkesSymbol) SymbolRisk() (engine.SymbolRisk, bool) {
	if !sym.hasFit || sym.fit.SpectralRadius <= 0 {
		return engine.SymbolRisk{}, false
	}

	return engine.SymbolRisk{
		SpectralRadius: sym.fit.SpectralRadius,
	}, true
}

func (sym *HawkesSymbol) Measure(
	ticks []trade.Data,
	snapshot engine.Snapshot,
	now time.Time,
	pair asset.Pair,
) (engine.Measurement, bool) {
	context, buyTimes, sellTimes, ok := fitContextFromTicks(ticks, time.Time{}, now)

	if !ok || !context.enoughEvents(buyTimes, sellTimes) {
		return engine.Measurement{}, false
	}

	fit := sym.FitBivariate(buyTimes, sellTimes, now)

	if fit.MuBuy <= 0 {
		return engine.Measurement{}, false
	}

	buyAsymmetry := intensityAsymmetry(fit, false)
	sellAsymmetry := intensityAsymmetry(fit, true)
	sellSide := sellAsymmetry > buyAsymmetry
	asymmetry := buyAsymmetry

	if sellSide {
		asymmetry = sellAsymmetry
	}

	raw := excitationConfidence(
		fit,
		asymmetry,
		sym.baselineIntensityFence(),
		sellSide,
	)

	bookSide := snapshot.Imbalance

	if sellSide {
		bookSide = math.Abs(snapshot.Imbalance)
	}

	scaled := errnie.Does(func() (float64, error) {
		return sym.gauge.Push(raw, math.Min(bookSide, 1))
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	confidence := sym.gaugeScore(scaled, true)

	if confidence <= 0 {
		return engine.Measurement{}, false
	}

	return engine.Measurement{
		Type: logic.Or(
			engine.Momentum,
			engine.Dump,
			sellSide,
		),
		Source: "hawkes",
		Regime: logic.Or(
			"momentum",
			"dump",
			sellSide,
		),
		Reason: logic.Or(
			"cluster_buy",
			"cluster_sell",
			sellSide,
		),
		Pairs:      []asset.Pair{pair},
		Confidence: confidence,
	}, true
}

func (sym *HawkesSymbol) recordConfidence(confidence float64) {
	if confidence <= 0 {
		return
	}

	capacity := confidenceHistoryCap(sym.minFitEvents)
	sym.confidenceHistory = append(sym.confidenceHistory, confidence)

	if len(sym.confidenceHistory) > capacity {
		sym.confidenceHistory = sym.confidenceHistory[len(sym.confidenceHistory)-capacity:]
	}
}

func (sym *HawkesSymbol) recordIntensityRatio(ratio float64) {
	if ratio <= 0 {
		return
	}

	capacity := confidenceHistoryCap(sym.minFitEvents)
	sym.intensityRatios = append(sym.intensityRatios, ratio)

	if len(sym.intensityRatios) > capacity {
		sym.intensityRatios = sym.intensityRatios[len(sym.intensityRatios)-capacity:]
	}
}
