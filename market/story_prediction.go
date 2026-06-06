package market

import (
	"fmt"
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const (
	defaultPredictionHorizon = time.Minute
	defaultPredictionAlpha   = types.DefaultCategorySurpriseAlpha
)

type forwardForecast struct {
	source    types.SourceType
	at        time.Time
	price     float64
	predicted float64
}

type sourceFeedbackStats struct {
	mse     adaptive.AlphaEMA
	scale   adaptive.AlphaEMA
	samples int
}

type symbolReturnStats struct {
	scale adaptive.AlphaEMA
}

type forwardFeedback struct {
	horizon time.Duration
	alpha   float64
	pending map[string][]forwardForecast
	sources map[types.SourceType]*sourceFeedbackStats
	symbols map[string]*symbolReturnStats
}

func newForwardFeedback(horizon time.Duration, alpha float64) *forwardFeedback {
	if horizon <= 0 {
		horizon = defaultPredictionHorizon
	}

	if alpha <= 0 || alpha > 1 {
		alpha = defaultPredictionAlpha
	}

	return &forwardFeedback{
		horizon: horizon,
		alpha:   alpha,
		pending: make(map[string][]forwardForecast),
		sources: make(map[types.SourceType]*sourceFeedbackStats),
		symbols: make(map[string]*symbolReturnStats),
	}
}

func newForwardFeedbackFromConfig() *forwardFeedback {
	return newForwardFeedback(
		viper.GetDuration("story.prediction.horizon"),
		viper.GetFloat64("story.prediction.alpha"),
	)
}

func (feedback *forwardFeedback) Observe(measurement types.Measurement) error {
	if feedback == nil {
		return fmt.Errorf("story: forward feedback is nil")
	}

	if !feedback.accepts(measurement) {
		return nil
	}

	if err := feedback.settle(measurement.Symbol, measurement.At, measurement.Last); err != nil {
		return err
	}

	feedback.pending[measurement.Symbol] = append(
		feedback.pending[measurement.Symbol],
		forwardForecast{
			source:    measurement.Source,
			at:        measurement.At,
			price:     measurement.Last,
			predicted: measurement.Confidence,
		},
	)

	return nil
}

func (feedback *forwardFeedback) accepts(measurement types.Measurement) bool {
	if measurement.Source == types.SourceNone || measurement.Source == types.SourcePrediction {
		return false
	}

	if measurement.Symbol == "" || measurement.At.IsZero() || measurement.Last <= 0 {
		return false
	}

	return measurement.Confidence > 0
}

func (feedback *forwardFeedback) settle(
	symbol string,
	now time.Time,
	price float64,
) error {
	pending := feedback.pending[symbol]

	if len(pending) == 0 || price <= 0 {
		return nil
	}

	open := pending[:0]

	for _, forecast := range pending {
		if now.Sub(forecast.at) < feedback.horizon {
			open = append(open, forecast)
			continue
		}

		if forecast.price <= 0 {
			continue
		}

		actualReturn := math.Abs(math.Log(price / forecast.price))
		actual := feedback.actualIntensity(symbol, actualReturn)
		errorValue := actual - forecast.predicted
		squaredError := errorValue * errorValue

		if err := feedback.observeSource(forecast.source, forecast.predicted, actual, squaredError); err != nil {
			return err
		}
	}

	if len(open) == 0 {
		delete(feedback.pending, symbol)

		return nil
	}

	feedback.pending[symbol] = open

	return nil
}

func (feedback *forwardFeedback) actualIntensity(symbol string, actualReturn float64) float64 {
	stats := feedback.symbolStats(symbol)
	_ = stats.scale.Update(actualReturn, feedback.alpha)

	scale := stats.scale.Value()

	if actualReturn <= 0 || scale <= 0 {
		return 0
	}

	return types.UnitCompetitionMargin(actualReturn, scale)
}

func (feedback *forwardFeedback) observeSource(
	source types.SourceType,
	predicted float64,
	actual float64,
	squaredError float64,
) error {
	stats := feedback.sourceStats(source)
	scaleSample := 0.0

	if predicted > 0 {
		scaleSample = actual / predicted
	}

	if err := stats.mse.Update(squaredError, feedback.alpha); err != nil {
		return err
	}

	if err := stats.scale.Update(scaleSample, feedback.alpha); err != nil {
		return err
	}

	stats.samples++

	_, err := types.UpdateSourceFeedback(
		source,
		stats.mse.Value(),
		stats.scale.Value(),
		stats.samples,
	)

	return err
}

func (feedback *forwardFeedback) sourceStats(source types.SourceType) *sourceFeedbackStats {
	stats, ok := feedback.sources[source]

	if ok {
		return stats
	}

	stats = &sourceFeedbackStats{}
	feedback.sources[source] = stats

	return stats
}

func (feedback *forwardFeedback) symbolStats(symbol string) *symbolReturnStats {
	stats, ok := feedback.symbols[symbol]

	if ok {
		return stats
	}

	stats = &symbolReturnStats{}
	feedback.symbols[symbol] = stats

	return stats
}

func (story *Story) observePredictionFeedback(
	measurement types.Measurement,
) (types.Measurement, numeric.Telemetry, bool, error) {
	if story.forwardFeedback == nil || measurement.Source == types.SourcePrediction {
		return types.Measurement{}, numeric.Telemetry{}, false, nil
	}

	if err := story.forwardFeedback.Observe(measurement); err != nil {
		return types.Measurement{}, numeric.Telemetry{}, false, err
	}

	if measurement.Source == types.SourceNone || measurement.Last <= 0 {
		return types.Measurement{}, numeric.Telemetry{}, false, nil
	}

	observation := measurement.Confidence
	story.predictionCalibrator.Observe(observation)

	code, err := story.predictionCalibrator.Classifier.Code(observation)

	if err != nil {
		return types.Measurement{}, numeric.Telemetry{}, false, err
	}

	category := types.CategoryType(story.predictionCalibrator.Classifier.Label(code))
	confidence := story.predictionCalibrator.Classifier.Confidence(observation)
	prediction := types.Measurement{
		At:         measurement.At,
		Symbol:     measurement.Symbol,
		Source:     types.SourcePrediction,
		Category:   category,
		Strength:   observation,
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
		return types.Measurement{}, numeric.Telemetry{}, false, err
	}

	return prediction, story.predictionCalibrator.Telemetry(observation), true, nil
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
