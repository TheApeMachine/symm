package prediction

import (
	"container/ring"
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/errnie"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/numeric"
)

var featureSources = []logic.SourceType{
	logic.SourceFluid,
	logic.SourceHawkes,
	logic.SourcePumpDump,
	logic.SourceDepthFlow,
	logic.SourceSentiment,
	logic.SourceCorrelation,
	logic.SourceCausal,
	logic.SourceLeadLag,
	logic.SourceLiquidity,
	logic.SourceExhaustion,
	logic.SourceCVD,
	logic.SourceToxicity,
}

type pendingForecast struct {
	matureAt      time.Time
	anchorPrice   float64
	forecast      float64
	features      []float64
	movementScale float64
}

/*
Signal attempts to predict the future movement of a symbol based
on past behavior, market conditions, and other factors. It will
store these predictions until the current time has caught up to
it and then calculate the error between the prediction and the actual
movement. This error is what is used as the feedback passed into the
Measurement Tuning System that every other signal in the system uses.
*/
type Signal struct {
	symbol               string
	entity               *logic.Entity
	measurements         *ring.Ring
	warmupRemaining      int
	horizon              time.Duration
	learningRate         float64
	learner              *numeric.RLSFilter
	features             []float64
	pending              []*pendingForecast
	lastResidual         float64
	feedbackSamples      int
	feedbackMSE          float64
	feedbackBias         float64
	realizedMagnitudeEMA float64
	chartEvents          ChartEvents
}

func NewSignal(
	symbol string,
	entity *logic.Entity,
	capacity int,
	horizon time.Duration,
	learningRate float64,
	initialVariance float64,
) (*Signal, error) {
	if horizon <= 0 {
		return nil, fmt.Errorf("prediction: horizon must be positive")
	}

	if learningRate <= 0 {
		return nil, fmt.Errorf("prediction: learning rate must be positive")
	}

	if initialVariance <= 0 {
		return nil, fmt.Errorf("prediction: rls initial variance must be positive")
	}

	featureCount := len(featureSources)
	learner, learnerErr := numeric.NewRLSFilter(featureCount, initialVariance)

	if learnerErr != nil {
		return nil, learnerErr
	}

	return &Signal{
		symbol:          symbol,
		entity:          entity,
		measurements:    ring.New(capacity),
		warmupRemaining: capacity,
		horizon:         horizon,
		learningRate:    learningRate,
		learner:         learner,
		features:        make([]float64, featureCount),
	}, nil
}

func (signal *Signal) Measure(_ *market.Feedback, at time.Time) (logic.Measurement, error) {
	switch signal.entity.Type {
	case logic.EntityMeasurement:
		return signal.measureMeasurement(at)
	case logic.EntityTrade:
		return signal.measureTrade(at)
	case logic.EntityTick:
		return signal.measureTick(at)
	case logic.EntityBook:
		return signal.measureBook(at)
	default:
		return logic.Measurement{}, errnie.Error(
			fmt.Errorf("prediction: unsupported entity %d", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureMeasurement(at time.Time) (logic.Measurement, error) {
	var err error

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		_, measurementOK := item.(logic.Measurement)

		if !measurementOK {
			err = fmt.Errorf("prediction: expected measurement")
		}
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	signal.rebuildFeaturesFromRing()

	return logic.Measurement{Symbol: signal.symbol, ObservedAt: at}, nil
}

func (signal *Signal) measureTrade(at time.Time) (logic.Measurement, error) {
	var (
		prices  []float64
		volumes []float64
		err     error
	)

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		trade, ok := item.(*krakenmarket.TradeUpdate)

		if !ok {
			err = fmt.Errorf("prediction: expected trade update")
			return
		}

		if trade.Price <= 0 || trade.Qty <= 0 {
			return
		}

		prices = append(prices, trade.Price)
		volumes = append(volumes, trade.Qty)
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	return signal.fromSeries(prices, volumes, nil, true, at)
}

func (signal *Signal) measureTick(at time.Time) (logic.Measurement, error) {
	var (
		prices  []float64
		volumes []float64
		spreads []float64
		err     error
	)

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		tick, ok := item.(*krakenmarket.TickerUpdate)

		if !ok {
			err = fmt.Errorf("prediction: expected ticker update")
			return
		}

		if tick.Bid <= 0 || tick.Ask <= tick.Bid {
			return
		}

		prices = append(prices, (tick.Ask+tick.Bid)/2)
		volumes = append(volumes, tick.AskQty+tick.BidQty)
		spreads = append(spreads, tick.Ask-tick.Bid)
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	return signal.fromSeries(prices, volumes, spreads, false, at)
}

func (signal *Signal) measureBook(at time.Time) (logic.Measurement, error) {
	var (
		prices  []float64
		volumes []float64
		spreads []float64
		err     error
	)

	folded := krakenmarket.Book{}

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		frame, ok := item.(*krakenmarket.Book)

		if !ok {
			err = fmt.Errorf("prediction: expected book update")
			return
		}

		folded.Fold(*frame, 0)

		mid, spread, depth, touchOK := folded.TouchQuote()

		if !touchOK || spread <= 0 {
			return
		}

		prices = append(prices, mid)
		volumes = append(volumes, depth)
		spreads = append(spreads, spread)
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	return signal.fromSeries(prices, volumes, spreads, false, at)
}

func (signal *Signal) fromSeries(
	prices []float64,
	volumes []float64,
	spreads []float64,
	forecastLoop bool,
	at time.Time,
) (logic.Measurement, error) {
	if len(prices) == 0 {
		return logic.Measurement{Symbol: signal.symbol, ObservedAt: at}, nil
	}

	if at.IsZero() {
		return logic.Measurement{}, errnie.Error(fmt.Errorf("prediction: event time is zero"))
	}

	price := prices[len(prices)-1]
	anchorPrice := numeric.Median(prices)
	settlementPrice := anchorPrice
	volume := numeric.Sum(volumes)
	spread := 0.0

	if len(spreads) > 0 {
		spread = spreads[len(spreads)-1]
	}

	forecast, predictErr := signal.predict(signal.features)

	if predictErr != nil {
		return logic.Measurement{}, errnie.Error(predictErr)
	}

	if forecastLoop {
		movementScale := signal.movementScale(prices)
		settlements, settleErr := signal.settlePending(at, settlementPrice)

		if settleErr != nil {
			return logic.Measurement{}, errnie.Error(settleErr)
		}

		signal.enqueueForecast(at, anchorPrice, forecast, movementScale)

		chartEvents := ChartEvents{
			Settlements: settlements,
		}

		if movementScale > 0 {
			chartEvents.ForecastTarget = float64(at.Add(signal.horizon).Unix())
			chartEvents.Forecast = signal.movementUnits(forecast, movementScale)
			chartEvents.HasForecast = true
		}

		signal.chartEvents = chartEvents
	}

	confidence := signal.movementConfidence(forecast, prices)
	strength := confidence

	position := logic.PositionTypeNone

	if forecast > 0 {
		position = logic.PositionTypeLong
	}

	if forecast < 0 {
		position = logic.PositionTypeShort
	}

	return logic.Measurement{
		Source:     logic.SourcePrediction,
		Symbol:     signal.symbol,
		Price:      price,
		Strength:   strength,
		Volume:     volume,
		Spread:     spread,
		Elapsed:    signal.horizon.Seconds(),
		Category:   logic.CategoryTypeNone,
		Regime:     logic.RegimeTypeNone,
		Position:   position,
		Confidence: confidence,
		Surprise:   math.Abs(signal.lastResidual),
		ObservedAt: at,
	}, nil
}

func (signal *Signal) Features() []float64 {
	return append([]float64(nil), signal.features...)
}

func (signal *Signal) ApplyFeatures(features []float64) {
	if len(features) != len(signal.features) {
		return
	}

	copy(signal.features, features)
}

func (signal *Signal) DrainChartEvents() ChartEvents {
	events := signal.chartEvents
	signal.chartEvents = ChartEvents{}

	return events
}

func (signal *Signal) DrainFeedback() *market.Feedback {
	if signal.feedbackSamples <= 0 {
		return nil
	}

	scale := 1.0

	if signal.feedbackMSE > 0 {
		scale = 1.0 / math.Sqrt(signal.feedbackMSE)
	}

	feedback := market.NewFeedback(
		signal.symbol,
		signal.feedbackMSE/float64(signal.feedbackSamples),
		scale,
		signal.feedbackBias/float64(signal.feedbackSamples),
		signal.feedbackSamples,
	)

	signal.feedbackSamples = 0
	signal.feedbackMSE = 0
	signal.feedbackBias = 0

	return feedback
}

func (signal *Signal) predict(features []float64) (float64, error) {
	return signal.learner.Predict(features)
}

func (signal *Signal) enqueueForecast(
	now time.Time,
	anchorPrice float64,
	forecast float64,
	movementScale float64,
) {
	capacity := signal.measurements.Len()

	if capacity <= 0 {
		return
	}

	features := append([]float64(nil), signal.features...)

	signal.pending = append(signal.pending, &pendingForecast{
		matureAt:      now.Add(signal.horizon),
		anchorPrice:   anchorPrice,
		forecast:      forecast,
		features:      features,
		movementScale: movementScale,
	})

	if len(signal.pending) > capacity {
		signal.pending = signal.pending[len(signal.pending)-capacity:]
	}
}

func (signal *Signal) settlePending(
	now time.Time,
	currentPrice float64,
) ([]ChartSettlement, error) {
	remaining := make([]*pendingForecast, 0, len(signal.pending))
	settlements := make([]ChartSettlement, 0, len(signal.pending))

	for _, pending := range signal.pending {
		if pending == nil {
			continue
		}

		if now.Before(pending.matureAt) {
			remaining = append(remaining, pending)
			continue
		}

		if pending.anchorPrice <= 0 || currentPrice <= 0 {
			continue
		}

		realized, realizedMagnitude := numeric.AnchorChange(
			pending.anchorPrice,
			currentPrice,
		)
		residual := realized - pending.forecast

		signal.updateRealizedMagnitude(realizedMagnitude)
		if learnErr := signal.learn(pending.features, realized); learnErr != nil {
			return nil, learnErr
		}
		signal.lastResidual = residual
		signal.feedbackSamples++
		signal.feedbackMSE += residual * residual
		signal.feedbackBias += residual

		if pending.movementScale <= 0 {
			continue
		}

		settlements = append(settlements, ChartSettlement{
			TargetUnix: float64(pending.matureAt.Unix()),
			Forecast: signal.movementUnits(
				pending.forecast,
				pending.movementScale,
			),
			Actual: signal.movementUnits(
				realized,
				pending.movementScale,
			),
		})
	}

	signal.pending = remaining

	return settlements, nil
}

func (signal *Signal) learn(features []float64, realized float64) error {
	target := realized / (1 + math.Abs(realized)/signal.scaledResidualScale())

	if observeErr := signal.learner.Observe(features, target); observeErr != nil {
		return observeErr
	}

	return nil
}

func (signal *Signal) scaledResidualScale() float64 {
	scale := signal.realizedMagnitudeEMA

	if scale <= 0 {
		scale = signal.featureIntensityBaseline()
	}

	return scale
}

func (signal *Signal) scaledResidual(residual float64) float64 {
	scale := signal.realizedMagnitudeEMA

	if scale <= 0 {
		scale = signal.featureIntensityBaseline()
	}

	if scale <= 0 {
		return residual
	}

	return residual / (1 + math.Abs(residual)/scale)
}

func (signal *Signal) updateRealizedMagnitude(magnitude float64) {
	if magnitude <= 0 {
		return
	}

	if signal.realizedMagnitudeEMA <= 0 {
		signal.realizedMagnitudeEMA = magnitude
		return
	}

	signal.realizedMagnitudeEMA = (1-signal.learningRate)*signal.realizedMagnitudeEMA +
		signal.learningRate*magnitude
}

func (signal *Signal) movementScale(prices []float64) float64 {
	if signal.realizedMagnitudeEMA <= 0 {
		return 0
	}

	spanScale := spanReturnScale(prices)

	if spanScale <= 0 {
		return signal.realizedMagnitudeEMA
	}

	scale := signal.realizedMagnitudeEMA

	if spanScale > scale {
		scale = spanScale
	}

	return scale
}

func (signal *Signal) movementUnits(value, scale float64) float64 {
	if scale <= 0 || value == 0 {
		return 0
	}

	sign := 1.0

	if value < 0 {
		sign = -1
	}

	forwardScore := math.Abs(value) / scale
	probabilities := numeric.SoftmaxScores([]float64{forwardScore, 1.0})

	if len(probabilities) == 0 {
		return 0
	}

	return sign * probabilities[0]
}

func (signal *Signal) movementConfidence(forecast float64, prices []float64) float64 {
	scale := signal.movementScale(prices)

	if scale <= 0 {
		return 0
	}

	return math.Abs(signal.movementUnits(forecast, scale))
}

func spanReturnScale(prices []float64) float64 {
	if len(prices) < 2 {
		return 0
	}

	if prices[0] <= 0 || prices[len(prices)-1] <= 0 {
		return 0
	}

	_, magnitude := numeric.AnchorChange(prices[0], prices[len(prices)-1])

	return magnitude
}

func (signal *Signal) featureIntensityBaseline() float64 {
	featureSum := 0.0
	featureCount := 0

	for _, feature := range signal.features {
		if feature <= 0 {
			continue
		}

		featureSum += feature
		featureCount++
	}

	if featureCount == 0 {
		return 1
	}

	return featureSum / float64(featureCount)
}

func (signal *Signal) Record(raw any) bool {
	warmed := false

	if signal.warmupRemaining > 0 {
		signal.warmupRemaining--
		warmed = true
	}

	signal.measurements.Value = raw
	signal.measurements = signal.measurements.Next()

	return warmed
}

func (signal *Signal) rebuildFeaturesFromRing() {
	for featureIndex := range signal.features {
		signal.features[featureIndex] = 0
	}

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		measurement, measurementOK := item.(logic.Measurement)

		if !measurementOK {
			return
		}

		sourceIndex := featureSourceIndex(measurement.Source)

		if sourceIndex < 0 {
			return
		}

		signal.features[sourceIndex] = measurement.Confidence
	})
}

func (signal *Signal) WarmupFilled() int {
	return signal.measurements.Len() - signal.warmupRemaining
}

func featureSourceIndex(source logic.SourceType) int {
	for index, featureSource := range featureSources {
		if featureSource == source {
			return index
		}
	}

	return -1
}
