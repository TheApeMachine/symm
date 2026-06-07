package market

import (
	"fmt"
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const (
	defaultPredictionHorizon = time.Minute
	defaultPredictionAlpha   = types.DefaultCategorySurpriseAlpha

	// predictionLearningRate is the online SGD step for the fusion weights —
	// small enough that one lucky settle cannot swing a source's weight, large
	// enough that an hour of settles converges.
	predictionLearningRate = 0.05
	// predictionWeightClamp bounds any single source's fusion weight.
	predictionWeightClamp = 1.0
	// predictionReturnClamp bounds the fused 60s log-return forecast (±5%).
	predictionReturnClamp = 0.05
	// predictionPendingCap bounds unsettled forecasts per symbol.
	predictionPendingCap = 64
)

type predictionChartKind string

const (
	predictionChartActual   predictionChartKind = "actual"
	predictionChartForecast predictionChartKind = "prediction"
	predictionChartError    predictionChartKind = "error"
)

/*
contribution is one line of a forecast's receipt: which source spoke and the
feature value (polarity × confidence) it contributed. The receipt is what makes
the error attributable — without it there is nothing to tune.
*/
type contribution struct {
	source types.SourceType
	x      float64
}

/*
pendingForecast is one fused prediction awaiting its ground truth: where the
price will be `horizon` from `at`, expressed as a signed log-return off basePrice.
*/
type pendingForecast struct {
	at        time.Time
	basePrice float64
	predicted float64 // signed log-return forecast over the horizon
	receipt   []contribution
}

type predictionChartPoint struct {
	kind  predictionChartKind
	at    time.Time
	value float64
}

// sourceStats accumulates one source's settled forecast quality: signed bias,
// squared error, and the magnitude calibration used to retune the signal's own
// observation scale (via types.AdjustSourceValue at the signal, never as a
// limiter on its outputs).
type sourceStats struct {
	mse         adaptive.AlphaEMA
	bias        adaptive.AlphaEMA
	absFeature  adaptive.AlphaEMA
	absRealized adaptive.AlphaEMA
	samples     int
}

type latestReading struct {
	x  float64
	at time.Time
}

/*
forwardFeedback is the closed loop the Signal interface was written for: the
story (where all signals come together) forms ONE fused prediction per symbol of
the price `horizon` from now, remembers the receipt of contributing signals, and
when ground truth catches up pushes the signed error back into per-source
calibration — tuning the values signals use to GENERATE confidence and surprise.
*/
type forwardFeedback struct {
	clampedForecasts uint64 // forecasts that hit the return clamp — visible distortion, not silent
	horizon            time.Duration
	alpha              float64
	pending            map[string][]pendingForecast
	weights            map[types.SourceType]float64
	sources            map[types.SourceType]*sourceStats
	readings           map[string]map[types.SourceType]latestReading
	lastPredictedPrice map[string]float64
	globalAbsRealized  adaptive.AlphaEMA
}

func newForwardFeedback(horizon time.Duration, alpha float64) *forwardFeedback {
	if horizon <= 0 {
		horizon = defaultPredictionHorizon
	}

	if alpha <= 0 || alpha > 1 {
		alpha = defaultPredictionAlpha
	}

	return &forwardFeedback{
		horizon:            horizon,
		alpha:              alpha,
		pending:            make(map[string][]pendingForecast),
		weights:            make(map[types.SourceType]float64),
		sources:            make(map[types.SourceType]*sourceStats),
		readings:           make(map[string]map[types.SourceType]latestReading),
		lastPredictedPrice: make(map[string]float64),
	}
}

func newForwardFeedbackFromConfig() *forwardFeedback {
	return newForwardFeedback(
		viper.GetDuration("story.prediction.horizon"),
		viper.GetFloat64("story.prediction.alpha"),
	)
}

/*
formedForecast reports a freshly formed fused prediction back to the story so it
can emit the prediction measurement and chart frames.
*/
type formedForecast struct {
	predictedReturn float64
	predictedPrice  float64
	at              time.Time
}

/*
Observe folds one real measurement into the loop: update the symbol's per-source
reading, settle any forecasts whose horizon has matured against this price, and —
at most once per distinct price — form the next fused forecast. Returns chart
points (predicted price, realized price, signed error) and the formed forecast
when one was created this call.
*/
func (feedback *forwardFeedback) Observe(
	measurement types.Measurement,
) ([]predictionChartPoint, *formedForecast, error) {
	if feedback == nil {
		return nil, nil, fmt.Errorf("story: forward feedback is nil")
	}

	if !feedback.accepts(measurement) {
		return nil, nil, nil
	}

	symbol := measurement.Symbol
	feedback.rememberReading(measurement)

	points, err := feedback.settle(symbol, measurement.At, measurement.Last)

	if err != nil {
		return nil, nil, err
	}

	// One forecast per distinct price: the prediction stream follows the
	// symbol's clock, not every source's. (The previous design mirrored every
	// measurement 1:1 — half of every capture was prediction rows.)
	if feedback.lastPredictedPrice[symbol] == measurement.Last {
		return points, nil, nil
	}

	feedback.lastPredictedPrice[symbol] = measurement.Last

	forecast := feedback.form(symbol, measurement.At, measurement.Last)

	if forecast == nil {
		return points, nil, nil
	}

	forecastPoint, err := newPredictionChartPoint(
		predictionChartForecast,
		measurement.At.Add(feedback.horizon),
		forecast.predictedPrice,
	)

	if err != nil {
		return nil, nil, err
	}

	return append(points, forecastPoint), forecast, nil
}

func (feedback *forwardFeedback) accepts(measurement types.Measurement) bool {
	if measurement.Source == types.SourceNone || measurement.Source == types.SourcePrediction {
		return false
	}

	return measurement.Symbol != "" && !measurement.At.IsZero() && measurement.Last > 0
}

func (feedback *forwardFeedback) rememberReading(measurement types.Measurement) {
	readings := feedback.readings[measurement.Symbol]

	if readings == nil {
		readings = make(map[types.SourceType]latestReading)
		feedback.readings[measurement.Symbol] = readings
	}

	readings[measurement.Source] = latestReading{
		x:  types.CategoryPolarity(measurement.Category) * measurement.Confidence,
		at: measurement.At,
	}
}

/*
form builds the fused forecast from every source reading fresh within two
horizons: predicted log-return = Σ weight(source) × feature(source).
*/
func (feedback *forwardFeedback) form(
	symbol string,
	at time.Time,
	price float64,
) *formedForecast {
	readings := feedback.readings[symbol]

	if len(readings) == 0 {
		return nil
	}

	staleBefore := at.Add(-2 * feedback.horizon)
	receipt := make([]contribution, 0, len(readings))
	predicted := 0.0

	for source, reading := range readings {
		if reading.at.Before(staleBefore) {
			continue
		}

		predicted += feedback.weights[source] * reading.x
		receipt = append(receipt, contribution{source: source, x: reading.x})
	}

	if len(receipt) == 0 {
		return nil
	}

	// The clamp guards the SGD learner from poisoning itself on one absurd
	// forecast; it is telemetry-visible rather than silent so a model that
	// LIVES at the clamp (predicting ±5% per minute, every minute) is a
	// finding on the dashboard, not a hidden distortion.
	if predicted > predictionReturnClamp || predicted < -predictionReturnClamp {
		feedback.clampedForecasts++

		if predicted > predictionReturnClamp {
			predicted = predictionReturnClamp
		} else {
			predicted = -predictionReturnClamp
		}
	}

	queue := append(feedback.pending[symbol], pendingForecast{
		at:        at,
		basePrice: price,
		predicted: predicted,
		receipt:   receipt,
	})

	if len(queue) > predictionPendingCap {
		queue = queue[len(queue)-predictionPendingCap:]
	}

	feedback.pending[symbol] = queue

	return &formedForecast{
		predictedReturn: predicted,
		predictedPrice:  price * math.Exp(predicted),
		at:              at,
	}
}

/*
settle matures forecasts whose horizon has elapsed: signed error in log-return
terms flows into the fusion weights (SGD through the receipt) and into each
contributing source's bias/mse/scale calibration.
*/
func (feedback *forwardFeedback) settle(
	symbol string,
	now time.Time,
	price float64,
) ([]predictionChartPoint, error) {
	pending := feedback.pending[symbol]

	if len(pending) == 0 || price <= 0 {
		return nil, nil
	}

	open := pending[:0]
	var points []predictionChartPoint

	for _, forecast := range pending {
		if now.Sub(forecast.at) < feedback.horizon {
			open = append(open, forecast)
			continue
		}

		if forecast.basePrice <= 0 {
			continue
		}

		realized := math.Log(price / forecast.basePrice)
		errorValue := realized - forecast.predicted

		feedback.learn(forecast, realized, errorValue)

		settledPoints, err := predictionChartSettledPoints(
			forecast.at.Add(feedback.horizon),
			price,
			100*errorValue, // signed error in percent
		)

		if err != nil {
			return nil, err
		}

		points = append(points, settledPoints...)
	}

	if len(open) == 0 {
		delete(feedback.pending, symbol)
	} else {
		feedback.pending[symbol] = open
	}

	return points, nil
}

func (feedback *forwardFeedback) learn(
	forecast pendingForecast,
	realized float64,
	errorValue float64,
) {
	absRealized := math.Abs(realized)
	_ = feedback.globalAbsRealized.Update(absRealized, feedback.alpha)
	movementUnit := feedback.globalAbsRealized.Value()

	for _, entry := range forecast.receipt {
		weight := feedback.weights[entry.source] + predictionLearningRate*errorValue*entry.x

		if weight > predictionWeightClamp {
			weight = predictionWeightClamp
		}

		if weight < -predictionWeightClamp {
			weight = -predictionWeightClamp
		}

		feedback.weights[entry.source] = weight

		stats := feedback.sourceStats(entry.source)
		err := stats.mse.Update(errorValue*errorValue, feedback.alpha)

		if err != nil {
			errnie.Error(err)
			continue
		}

		err = stats.bias.Update(errorValue, feedback.alpha)

		if err != nil {
			errnie.Error(err)
			continue
		}

		err = stats.absFeature.Update(math.Abs(entry.x), feedback.alpha)

		if err != nil {
			errnie.Error(err)
			continue
		}

		err = stats.absRealized.Update(absRealized, feedback.alpha)

		if err != nil {
			errnie.Error(err)
			continue
		}

		stats.samples++

		scale := 1.0

		if stats.absFeature.Value() > 0 && movementUnit > 0 {
			// How big moves actually are when this source speaks, against how
			// loudly it spoke, normalized by the market's typical move: >1 means
			// the source under-calls (sharpen its observation scale), <1 means it
			// over-calls (soften). Consumed by signals via AdjustSourceValue —
			// tuning the value confidence and surprise derive from.
			scale = (stats.absRealized.Value() / stats.absFeature.Value()) / movementUnit
		}

		if math.IsNaN(scale) || math.IsInf(scale, 0) || scale < 0 {
			scale = 1
		}

		_, err = types.UpdateSourceFeedback(
			entry.source,
			stats.mse.Value(),
			scale,
			stats.bias.Value(),
			stats.samples,
		)

		if err != nil {
			errnie.Error(err)
			continue
		}
	}
}

func (feedback *forwardFeedback) sourceStats(source types.SourceType) *sourceStats {
	stats, ok := feedback.sources[source]

	if ok {
		return stats
	}

	stats = &sourceStats{}
	feedback.sources[source] = stats

	return stats
}

func newPredictionChartPoint(
	kind predictionChartKind,
	at time.Time,
	value float64,
) (predictionChartPoint, error) {
	if at.IsZero() {
		return predictionChartPoint{}, fmt.Errorf("story: prediction chart timestamp is zero")
	}

	if math.IsNaN(value) || math.IsInf(value, 0) {
		return predictionChartPoint{}, fmt.Errorf(
			"story: prediction chart value is invalid: %v",
			value,
		)
	}

	return predictionChartPoint{kind: kind, at: at, value: value}, nil
}

func predictionChartSettledPoints(
	at time.Time,
	realizedPrice float64,
	errorPercent float64,
) ([]predictionChartPoint, error) {
	actualPoint, err := newPredictionChartPoint(predictionChartActual, at, realizedPrice)

	if err != nil {
		return nil, errnie.Error(err)
	}

	errorPoint, err := newPredictionChartPoint(predictionChartError, at, errorPercent)

	if err != nil {
		return nil, errnie.Error(err)
	}

	return []predictionChartPoint{actualPoint, errorPoint}, nil
}

func (point predictionChartPoint) Payload(symbol string) map[string]any {
	return map[string]any{
		"chart":  "prediction",
		"symbol": symbol,
		"kind":   string(point.kind),
		"x":      float64(point.at.UnixNano()) / float64(time.Second),
		"value":  point.value,
	}
}

/*
observePredictionFeedback runs the fused prediction loop for one real
measurement. When a new forecast was formed it returns the prediction
measurement to record and remember: Strength carries the SIGNED predicted
log-return; the category bands |forecast| through the self-calibrating
classifier so playbooks can keep gating on prediction categories.
*/
func (story *Story) observePredictionFeedback(
	measurement types.Measurement,
) (types.Measurement, numeric.Telemetry, []predictionChartPoint, bool, error) {
	if story.forwardFeedback == nil || measurement.Source == types.SourcePrediction {
		return types.Measurement{}, numeric.Telemetry{}, nil, false, nil
	}

	points, forecast, err := story.forwardFeedback.Observe(measurement)

	if err != nil {
		return types.Measurement{}, numeric.Telemetry{}, nil, false, errnie.Error(err)
	}

	if forecast == nil {
		return types.Measurement{}, numeric.Telemetry{}, points, false, nil
	}

	observation := math.Abs(forecast.predictedReturn)
	story.predictionCalibrator.Observe(observation)

	code, err := story.predictionCalibrator.Classifier.Code(observation)

	if err != nil {
		return types.Measurement{}, numeric.Telemetry{}, nil, false, errnie.Error(err)
	}

	category := types.CategoryType(story.predictionCalibrator.Classifier.Label(code))
	confidence := story.predictionCalibrator.Classifier.Confidence(observation)
	prediction := types.Measurement{
		At:         measurement.At,
		Symbol:     measurement.Symbol,
		Source:     types.SourcePrediction,
		Category:   category,
		Strength:   forecast.predictedReturn, // SIGNED 60s log-return forecast
		Confidence: confidence,
		Last:       measurement.Last,
		Volume:     measurement.Volume,
		SpreadBPS:  measurement.SpreadBPS,
		Bid:        measurement.Bid,
		Ask:        measurement.Ask,
	}

	if err := types.AssignCategorySurpriseSNR(
		&prediction, story.predictionSurpriseField, category,
	); err != nil {
		return types.Measurement{}, numeric.Telemetry{}, nil, false, errnie.Error(err)
	}

	return prediction, story.predictionCalibrator.Telemetry(observation), points, true, nil
}

func (story *Story) publishPredictionGauge(
	measurement types.Measurement,
	telemetry numeric.Telemetry,
) {
	if story.ui == nil {
		return
	}

	story.ui.Send(&qpool.QValue[any]{
		Value: numeric.GaugePayload(
			measurement.Source.String(),
			measurement.Symbol,
			measurement.Category,
			measurement,
			telemetry,
		),
	})
}

func (story *Story) publishPredictionChart(
	symbol string,
	points []predictionChartPoint,
) {
	if story.ui == nil {
		return
	}

	for _, point := range points {
		story.ui.Send(&qpool.QValue[any]{Value: point.Payload(symbol)})
	}
}
