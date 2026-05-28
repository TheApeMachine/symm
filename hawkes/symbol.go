package hawkes

import (
	"math"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/trade"
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
	lastFitEventKey   fitEventKey
	lastFitTime       time.Time
	fitCooldown       time.Duration
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
		fitCooldown:       config.System.HawkesFitCooldown,
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
	sym.lastFitEventKey = fitEventKey{}
	sym.lastFitTime = time.Time{}
}

func (sym *HawkesSymbol) FitBivariate(
	stream ArrivalStream,
	horizon time.Time,
) BivariateFit {
	prior := BivariateFit{}

	if sym.hasFit {
		prior = sym.fit.Calibrated(sym.calibrator.Scale())
	}

	context, ok := NewFitContext(stream, horizon)

	if ok {
		sym.minFitEvents = context.MinFitEvents
	}

	fit := NewBivariateEstimator(prior).Fit(stream, horizon)

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

func (sym *HawkesSymbol) refreshFitIntensities(
	stream ArrivalStream,
	horizon time.Time,
) BivariateFit {
	return sym.fit.WithIntensitiesAt(stream, horizon)
}

func (sym *HawkesSymbol) fitForEvents(
	stream ArrivalStream,
	horizon time.Time,
) (BivariateFit, bool) {
	key := stream.RevisionKey()

	if sym.hasFit && key == sym.lastFitEventKey {
		return sym.refreshFitIntensities(stream, horizon), true
	}

	if sym.hasFit &&
		sym.fitCooldown > 0 &&
		!sym.lastFitTime.IsZero() &&
		horizon.Sub(sym.lastFitTime) < sym.fitCooldown {
		return sym.refreshFitIntensities(stream, horizon), true
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
	ticks []trade.Data,
	imbalance float64,
	now time.Time,
	pair asset.Pair,
) (engine.Measurement, bool) {
	context, stream, ok := FitContextFromTicks(ticks, time.Time{}, now)

	if !ok || !context.EnoughEvents(stream) {
		return engine.Measurement{}, false
	}

	fit, ok := sym.fitForEvents(stream, now)

	if !ok {
		return engine.Measurement{}, false
	}

	buyAsymmetry := fit.Asymmetry(false)
	sellAsymmetry := fit.Asymmetry(true)
	sellSide := sellAsymmetry > buyAsymmetry
	asymmetry := buyAsymmetry

	if sellSide {
		asymmetry = sellAsymmetry
	}

	bookSide := imbalance

	if sellSide {
		bookSide = math.Abs(imbalance)
	}

	confidence := sym.clusterConfidence(
		fit,
		asymmetry,
		bookSide,
		sellSide,
	)

	if confidence <= 0 {
		return engine.Measurement{}, false
	}

	runwaySec := int64(fit.Runway().Seconds())

	if runwaySec < 1 {
		runwaySec = 1
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
		Timeframe: engine.Timeframe{
			Start: now.Unix(),
			End:   now.Unix() + runwaySec,
		},
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

/*
clusterConfidence scores trade-cluster excitation against book confirmation from the
current fit and top-of-book imbalance.
*/
func (sym *HawkesSymbol) clusterConfidence(
	fit BivariateFit,
	asymmetry float64,
	bookSide float64,
	sellSide bool,
) float64 {
	if asymmetry <= 0 || fit.SpectralRadius >= criticalBranch {
		return 0
	}

	ratio := 0.0

	if sellSide {
		if fit.MuSell <= 0 || fit.SellIntensity <= 0 {
			return 0
		}

		ratio = fit.SellIntensity / fit.MuSell
	}

	if !sellSide {
		if fit.MuBuy <= 0 || fit.BuyIntensity <= 0 {
			return 0
		}

		ratio = fit.BuyIntensity / fit.MuBuy
	}

	fence := sym.baselineIntensityFence()

	if ratio <= fence {
		return 0
	}

	if fence <= 0 {
		fence = 1
	}

	cluster := asymmetry * engine.ExcessRatio(ratio/fence)
	side := math.Min(math.Abs(bookSide), 1)

	return engine.AlignConfidence(cluster, side)
}
