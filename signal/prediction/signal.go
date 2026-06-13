package prediction

import (
	"container/ring"
	"fmt"
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/statistic"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/numeric"
	signalsupport "github.com/theapemachine/symm/signal"
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
	logic.SourceManifold,
}

const learningTargetScaleFloor = 1e-4

type pendingForecast struct {
	matureAt      time.Time
	anchorPrice   float64
	forecast      float64
	features      []float64
	movementScale float64
	regime        predictionRegime
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
	forecastInterval     time.Duration
	lastForecastAt       time.Time
	learningRate         float64
	learner              *learning.RLSFilter
	features             []float64
	featureCategories    []logic.CategoryType
	featureRegimes       []logic.RegimeType
	pending              []*pendingForecast
	lastResidual         float64
	feedbackSamples      int
	feedbackMSE          float64
	feedbackBias         float64
	realizedMagnitudeEMA float64
	chartEvents          ChartEvents
	chart                *Chart
}

func NewSignal(
	symbol string,
	entity *logic.Entity,
	chart *Chart,
) *Signal {
	capacity := market.MustSignalMeasurementCapacity()

	horizon := viper.GetDuration("story.prediction.horizon")
	forecastInterval := viper.GetDuration("story.prediction.interval")

	learningRate := math.Min(
		math.Max(viper.GetFloat64("story.prediction.alpha"), 0.01),
		1.0,
	)
	initialVariance := viper.GetFloat64("story.prediction.rls_initial_variance")

	if initialVariance <= 0 {
		initialVariance = 1.0
	}

	featureCount := len(featureSources)
	learner, _ := learning.NewRLSFilter(featureCount, initialVariance)

	return &Signal{
		symbol:           symbol,
		entity:           entity,
		chart:            chart,
		measurements:     ring.New(capacity),
		warmupRemaining:  capacity,
		horizon:          horizon,
		forecastInterval: forecastInterval,
		learningRate:     learningRate,
		learner:          learner,
		features:         make([]float64, featureCount),
		featureCategories: make(
			[]logic.CategoryType,
			featureCount,
		),
		featureRegimes: make(
			[]logic.RegimeType,
			featureCount,
		),
	}
}

func (signal *Signal) Symbol() string {
	return signal.symbol
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
		return logic.Measurement{}, nil
	}
}

func (signal *Signal) withheldMeasurement(at time.Time) logic.Measurement {
	return logic.Measurement{
		Source:     logic.SourcePrediction,
		Symbol:     signal.symbol,
		Category:   logic.CategoryTypeNone,
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		ObservedAt: at,
	}
}

func (signal *Signal) measureMeasurement(_ time.Time) (logic.Measurement, error) {
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

	return logic.Measurement{}, nil
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

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		frame, ok := item.(*krakenmarket.BookUpdate)

		if !ok {
			err = fmt.Errorf("prediction: expected book update")
			return
		}

		if len(frame.Bids) == 0 || len(frame.Asks) == 0 {
			return
		}

		touchSpread := frame.Asks[0].Price - frame.Bids[0].Price

		if touchSpread <= 0 {
			return
		}

		spreads = append(spreads, touchSpread)

		for _, bid := range frame.Bids {
			if bid.Qty > 0 {
				prices = append(prices, bid.Price)
				volumes = append(volumes, bid.Qty)
			}
		}

		for _, ask := range frame.Asks {
			if ask.Qty > 0 {
				prices = append(prices, ask.Price)
				volumes = append(volumes, ask.Qty)
			}
		}
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
	if len(prices) < 2 {
		return logic.Measurement{}, nil
	}

	if signal.horizon <= 0 {
		return logic.Measurement{}, errnie.Error(fmt.Errorf("prediction: horizon is required"))
	}

	if at.IsZero() {
		return logic.Measurement{}, errnie.Error(fmt.Errorf("prediction: event time is zero"))
	}

	price := prices[len(prices)-1]
	anchorPrice := float64(statistic.NewMedian(nil).Observe(nomagique.Numbers(prices...)...))
	settlementPrice := anchorPrice
	volume := float64(statistic.NewSum().Observe(nomagique.Numbers(volumes...)...))

	if volume <= 0 {
		return logic.Measurement{}, nil
	}

	spread := 0.0

	if len(spreads) > 0 {
		spread = spreads[len(spreads)-1]
	}

	if spread <= 0 {
		var spreadErr error
		spread, spreadErr = signalsupport.TouchSpread(prices)

		if spreadErr != nil {
			return signal.withheldMeasurement(at), nil
		}
	}

	_, _, ok := signalsupport.ResolvedChange(prices)

	if !ok {
		return signal.withheldMeasurement(at), nil
	}

	row, err := krakenmarket.SymbolRowFromPrices(signal.symbol, prices, volume, 1, at)

	if err != nil {
		return logic.Measurement{}, nil
	}

	forecast, err := signal.resolveForecast(prices)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if forecast == 0 {
		return logic.Measurement{}, nil
	}

	if forecastLoop {
		movementScale := signal.movementScale(prices)
		settlements, err := signal.settlePending(at, settlementPrice)

		if err != nil {
			return logic.Measurement{}, errnie.Error(err)
		}

		chartEvents := ChartEvents{
			Settlements: settlements,
			EventAt:     at,
		}

		if signal.forecastAllowed(at) {
			normalizeScale := movementScale

			if normalizeScale <= 0 {
				normalizeScale = signal.scaledResidualScale()
			}

			if normalizeScale <= 0 {
				return logic.Measurement{}, errnie.Error(
					fmt.Errorf("prediction: movement scale must be positive"),
				)
			}

			signal.enqueueForecast(at, anchorPrice, forecast, normalizeScale)

			forecastUnits, unitsErr := signal.movementUnits(forecast, normalizeScale)

			if unitsErr != nil {
				return logic.Measurement{}, errnie.Error(unitsErr)
			}

			chartEvents.ForecastTarget = float64(at.Add(signal.horizon).Unix())
			chartEvents.Forecast = forecastUnits
			chartEvents.HasForecast = true
		}

		if chartEvents.HasForecast || len(chartEvents.Settlements) > 0 {
			signal.chartEvents = chartEvents
			signal.publishChartEvents()
		}
	}

	confidence, err := signal.movementConfidence(forecast, prices)

	if err != nil || confidence <= 0 {
		return logic.Measurement{}, nil
	}

	strength := confidence

	position := logic.PositionTypeNone

	if forecast > 0 {
		position = logic.PositionTypeLong
	}

	if forecast < 0 {
		position = logic.PositionTypeShort
	}

	surprise := signal.observationSurprise(prices)

	if surprise <= 0 {
		return logic.Measurement{}, nil
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
		Surprise:   surprise,
		ObservedAt: at,
		Market:     *row,
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

func (signal *Signal) publishChartEvents() {
	if signal.chart == nil {
		return
	}

	if !signal.chartEvents.HasForecast && len(signal.chartEvents.Settlements) == 0 {
		return
	}

	events := signal.chartEvents
	signal.chartEvents = ChartEvents{}

	errnie.Error(signal.chart.Apply(signal.symbol, events))
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

func (signal *Signal) resolveForecast(prices []float64) (float64, error) {
	forecast, err := signal.predict(signal.features)

	if err != nil {
		return 0, err
	}

	if forecast != 0 {
		return forecast, nil
	}

	baseline := signal.featureIntensityBaseline()

	if baseline <= 0 {
		return 0, nil
	}

	move, magnitude, ok := signalsupport.ResolvedChange(prices)

	if !ok || magnitude <= 0 {
		return 0, nil
	}

	return move * baseline, nil
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
		regime:        signal.currentRegime(),
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
		currentRegime := signal.currentRegime()
		regimeShifted := pending.regime.Shifted(currentRegime)
		panicShifted := pending.regime.Panic() != currentRegime.Panic()

		signal.updateRealizedMagnitude(realizedMagnitude)

		if !regimeShifted && !panicShifted {
			if learnErr := signal.learn(pending.features, realized); learnErr != nil {
				errnie.Error(learnErr)
			}

			signal.lastResidual = residual
			signal.feedbackSamples++
			signal.feedbackMSE += residual * residual
			signal.feedbackBias += residual
		}

		if pending.movementScale <= 0 {
			continue
		}

		forecastUnits, forecastUnitsErr := signal.movementUnits(
			pending.forecast,
			pending.movementScale,
		)

		if forecastUnitsErr != nil {
			return nil, forecastUnitsErr
		}

		actualUnits, err := signal.movementUnits(
			realized,
			pending.movementScale,
		)

		if err != nil {
			return nil, errnie.Error(err)
		}

		settlements = append(settlements, ChartSettlement{
			TargetUnix: float64(pending.matureAt.Unix()),
			Forecast:   forecastUnits,
			Actual:     actualUnits,
		})
	}

	signal.pending = remaining

	return settlements, nil
}

func (signal *Signal) forecastAllowed(at time.Time) bool {
	if signal.forecastInterval <= 0 {
		return true
	}

	if signal.lastForecastAt.IsZero() {
		signal.lastForecastAt = at
		return true
	}

	if at.Sub(signal.lastForecastAt) < signal.forecastInterval {
		return false
	}

	signal.lastForecastAt = at

	return true
}

func (signal *Signal) learningTarget(realized float64) float64 {
	scale := math.Max(signal.scaledResidualScale(), learningTargetScaleFloor)

	return realized / (1 + math.Abs(realized)/scale)
}

func (signal *Signal) learn(features []float64, realized float64) error {
	target := signal.learningTarget(realized)

	adaptation, err := market.LoadAdaptation()

	if err == nil {
		forgettingFactor := 1 - adaptation.Alpha()

		if forgettingFactor < 0.01 {
			forgettingFactor = 0.01
		}

		if setErr := signal.learner.SetForgettingFactor(forgettingFactor); setErr != nil {
			return errnie.Error(setErr)
		}
	}

	if err := signal.learner.Observe(features, target); err != nil {
		return errnie.Error(err)
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
	scale := signal.realizedMagnitudeEMA

	if spanScale > scale {
		scale = spanScale
	}

	return scale
}

func (signal *Signal) observationSurprise(prices []float64) float64 {
	if surprise := math.Abs(signal.lastResidual); surprise > 0 {
		return surprise
	}

	_, magnitude, ok := signalsupport.ResolvedChange(prices)

	if ok && magnitude > 0 {
		return magnitude
	}

	if len(prices) >= 2 {
		spread, spreadErr := signalsupport.TouchSpread(prices)

		if spreadErr == nil {
			price := prices[len(prices)-1]

			if price > 0 && spread > 0 {
				return spread / price
			}
		}
	}

	return signal.featureIntensityBaseline()
}

func (signal *Signal) movementUnits(value, scale float64) (float64, error) {
	if scale <= 0 || value == 0 {
		return 0, nil
	}

	sign := 1.0

	if value < 0 {
		sign = -1
	}

	forwardScore := math.Abs(value) / scale
	probabilities, err := probability.SoftmaxScores([]float64{forwardScore, 1.0})

	if err != nil {
		return 0, err
	}

	return sign * probabilities[0], nil
}

func (signal *Signal) movementConfidence(forecast float64, prices []float64) (float64, error) {
	scale := signal.movementScale(prices)

	if scale <= 0 {
		scale = signal.scaledResidualScale()
	}

	if scale <= 0 {
		return 0, fmt.Errorf("prediction: movement scale must be positive")
	}

	units, err := signal.movementUnits(forecast, scale)

	if err != nil {
		return 0, err
	}

	confidence := math.Abs(units)

	if confidence <= 0 {
		return 0, fmt.Errorf("prediction: confidence is required")
	}

	return confidence, nil
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
		return 0
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
		signal.featureCategories[featureIndex] = logic.CategoryTypeNone
		signal.featureRegimes[featureIndex] = logic.RegimeTypeNone
	}

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		measurement, measurementOK := item.(logic.Measurement)

		if !measurementOK {
			return
		}

		signal.recordFeatureMeasurement(measurement)
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
