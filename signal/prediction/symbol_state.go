package prediction

import (
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/nomagique/statistic"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type symbolState struct {
	warmupRemaining      int
	lastForecastAt       time.Time
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
}

func newSymbolState() *symbolState {
	featureCount := len(featureSources)
	initialVariance := 1.0

	learner, _ := learning.NewRLSFilter(featureCount, initialVariance)

	return &symbolState{
		warmupRemaining: measurementsCapacity(),
		learner:         learner,
		features:        make([]float64, featureCount),
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

func (state *symbolState) recordTrade() bool {
	warmed := false

	if state.warmupRemaining > 0 {
		state.warmupRemaining--
		warmed = true
	}

	return warmed
}

func (state *symbolState) warmupFilled() int {
	return measurementsCapacity() - state.warmupRemaining
}

func (state *symbolState) fromSeries(
	signal *Signal,
	symbol string,
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
	anchorPrice := statistic.MedianOf(prices)
	settlementPrice := anchorPrice
	volume := sumFloats(volumes)

	if volume <= 0 {
		return logic.Measurement{}, nil
	}

	spread := 0.0

	if len(spreads) > 0 {
		spread = spreads[len(spreads)-1]
	}

	if spread <= 0 {
		var spreadErr error
		spread, spreadErr = touchSpread(prices)

		if spreadErr != nil {
			return logic.Measurement{}, nil
		}
	}

	_, _, ok := resolvedChange(prices)

	if !ok {
		return logic.Measurement{}, nil
	}

	row, err := krakenmarket.SymbolRowFromPrices(symbol, prices, volume, 1, at)

	if err != nil {
		return logic.Measurement{}, nil
	}

	forecast, err := state.resolveForecast(prices)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if forecast == 0 {
		return logic.Measurement{}, nil
	}

	if forecastLoop {
		movementScale := state.movementScale(prices)
		settlements, settleErr := state.settlePending(signal, at, settlementPrice)

		if settleErr != nil {
			return logic.Measurement{}, errnie.Error(settleErr)
		}

		chartEvents := ChartEvents{
			Settlements: settlements,
			EventAt:     at,
		}

		if state.forecastAllowed(signal, at) {
			normalizeScale := movementScale

			if normalizeScale <= 0 {
				normalizeScale = state.scaledResidualScale()
			}

			if normalizeScale <= 0 {
				return logic.Measurement{}, errnie.Error(
					fmt.Errorf("prediction: movement scale must be positive"),
				)
			}

			state.enqueueForecast(signal, at, anchorPrice, forecast, normalizeScale)

			forecastUnits, unitsErr := movementUnits(forecast, normalizeScale)

			if unitsErr != nil {
				return logic.Measurement{}, errnie.Error(unitsErr)
			}

			chartEvents.ForecastTarget = float64(at.Add(signal.horizon).Unix())
			chartEvents.Forecast = forecastUnits
			chartEvents.HasForecast = true
		}

		if chartEvents.HasForecast || len(chartEvents.Settlements) > 0 {
			state.chartEvents = chartEvents
			state.publishChartEvents(signal, symbol)
		}
	}

	confidence, err := state.movementConfidence(forecast, prices)

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

	surprise := state.observationSurprise(prices)

	if surprise <= 0 {
		return logic.Measurement{}, nil
	}

	movementScale := state.movementScale(prices)
	expectedMoveBps := math.Abs(forecast) * 10000
	edgeSurprise := math.Abs(state.lastResidual) * 10000

	if movementScale > 0 {
		edgeSurprise = math.Abs(state.lastResidual) / movementScale * 10000
	}

	return logic.Measurement{
		Source:          logic.SourcePrediction,
		Symbol:          symbol,
		Price:           price,
		Strength:        strength,
		Volume:          volume,
		Spread:          spread,
		Elapsed:         signal.horizon.Seconds(),
		Category:        logic.CategoryForecastEdge,
		Regime:          logic.RegimeTypeNone,
		Position:        position,
		Confidence:      confidence,
		Surprise:        surprise,
		EdgeConfidence:  confidence,
		ExpectedMoveBps: expectedMoveBps,
		EdgeSurprise:    edgeSurprise,
		NoveltySurprise: surprise,
		ObservedAt:      at,
		Market:          row.Name,
	}.UnlessPublishable(), nil
}

func (state *symbolState) publishChartEvents(signal *Signal, symbol string) {
	if signal.chart == nil {
		return
	}

	if !state.chartEvents.HasForecast && len(state.chartEvents.Settlements) == 0 {
		return
	}

	events := state.chartEvents
	state.chartEvents = ChartEvents{}

	errnie.Error(signal.chart.Apply(symbol, events))
}

func (state *symbolState) drainChartEvents() ChartEvents {
	events := state.chartEvents
	state.chartEvents = ChartEvents{}

	return events
}

func (state *symbolState) drainFeedback() *market.Feedback {
	if state.feedbackSamples <= 0 {
		return nil
	}

	scale := 1.0

	if state.feedbackMSE > 0 {
		scale = 1.0 / math.Sqrt(state.feedbackMSE)
	}

	feedback := market.NewFeedback(
		"",
		state.feedbackMSE/float64(state.feedbackSamples),
		scale,
		state.feedbackBias/float64(state.feedbackSamples),
		state.feedbackSamples,
	)

	state.feedbackSamples = 0
	state.feedbackMSE = 0
	state.feedbackBias = 0

	return feedback
}

func (state *symbolState) predict(features []float64) (float64, error) {
	return state.learner.Predict(features)
}

func (state *symbolState) resolveForecast(prices []float64) (float64, error) {
	forecast, err := state.predict(state.features)

	if err != nil {
		return 0, err
	}

	if forecast != 0 {
		return forecast, nil
	}

	baseline := state.featureIntensityBaseline()

	if baseline <= 0 {
		return 0, nil
	}

	move, _, ok := resolvedChange(prices)

	if !ok || move == 0 {
		return 0, nil
	}

	return move * baseline, nil
}

func (state *symbolState) enqueueForecast(
	signal *Signal,
	now time.Time,
	anchorPrice float64,
	forecast float64,
	movementScale float64,
) {
	capacity := measurementsCapacity()

	if capacity <= 0 {
		return
	}

	features := append([]float64(nil), state.features...)

	state.pending = append(state.pending, &pendingForecast{
		matureAt:      now.Add(signal.horizon),
		anchorPrice:   anchorPrice,
		forecast:      forecast,
		features:      features,
		movementScale: movementScale,
		regime:        state.currentRegime(),
	})

	if len(state.pending) > capacity {
		state.pending = state.pending[len(state.pending)-capacity:]
	}
}

func (state *symbolState) settlePending(
	signal *Signal,
	now time.Time,
	currentPrice float64,
) ([]ChartSettlement, error) {
	remaining := make([]*pendingForecast, 0, len(state.pending))
	settlements := make([]ChartSettlement, 0, len(state.pending))

	for _, pending := range state.pending {
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

		realized, realizedMagnitude := anchorChange(
			pending.anchorPrice,
			currentPrice,
		)
		residual := realized - pending.forecast
		currentRegime := state.currentRegime()
		regimeShifted := pending.regime.Shifted(currentRegime)
		panicShifted := pending.regime.Panic() != currentRegime.Panic()

		state.updateRealizedMagnitude(signal, realizedMagnitude)

		if !regimeShifted && !panicShifted {
			if learnErr := state.learn(signal, pending.features, realized); learnErr != nil {
				errnie.Error(learnErr)
			}

			state.lastResidual = residual
			state.feedbackSamples++
			state.feedbackMSE += residual * residual
			state.feedbackBias += residual
		}

		if pending.movementScale <= 0 {
			continue
		}

		forecastUnits, forecastUnitsErr := movementUnits(
			pending.forecast,
			pending.movementScale,
		)

		if forecastUnitsErr != nil {
			return nil, forecastUnitsErr
		}

		actualUnits, actualUnitsErr := movementUnits(
			realized,
			pending.movementScale,
		)

		if actualUnitsErr != nil {
			return nil, errnie.Error(actualUnitsErr)
		}

		settlements = append(settlements, ChartSettlement{
			TargetUnix: float64(pending.matureAt.Unix()),
			Forecast:   forecastUnits,
			Actual:     actualUnits,
		})
	}

	state.pending = remaining

	return settlements, nil
}

func (state *symbolState) forecastAllowed(signal *Signal, at time.Time) bool {
	if signal.forecastInterval <= 0 {
		return true
	}

	if state.lastForecastAt.IsZero() {
		state.lastForecastAt = at
		return true
	}

	if at.Sub(state.lastForecastAt) < signal.forecastInterval {
		return false
	}

	state.lastForecastAt = at

	return true
}

func (state *symbolState) learningTarget(realized float64) float64 {
	scale := math.Max(state.scaledResidualScale(), learningTargetScaleFloor)

	return realized / (1 + math.Abs(realized)/scale)
}

func (state *symbolState) learn(
	signal *Signal,
	features []float64,
	realized float64,
) error {
	target := state.learningTarget(realized)
	factor := forgettingFactor()

	if setErr := state.learner.SetForgettingFactor(factor); setErr != nil {
		return errnie.Error(setErr)
	}

	_ = signal

	if err := state.learner.Observe(features, target); err != nil {
		return errnie.Error(err)
	}

	return nil
}

func (state *symbolState) scaledResidualScale() float64 {
	scale := state.realizedMagnitudeEMA

	if scale <= 0 {
		scale = state.featureIntensityBaseline()
	}

	return scale
}

func (state *symbolState) updateRealizedMagnitude(signal *Signal, magnitude float64) {
	if magnitude <= 0 {
		return
	}

	if state.realizedMagnitudeEMA <= 0 {
		state.realizedMagnitudeEMA = magnitude
		return
	}

	state.realizedMagnitudeEMA = (1-signal.learningRate)*state.realizedMagnitudeEMA +
		signal.learningRate*magnitude
}

func (state *symbolState) movementScale(prices []float64) float64 {
	if state.realizedMagnitudeEMA <= 0 {
		return 0
	}

	spanScale := spanReturnScale(prices)
	scale := state.realizedMagnitudeEMA

	if spanScale > scale {
		scale = spanScale
	}

	return scale
}

func (state *symbolState) observationSurprise(prices []float64) float64 {
	if surprise := math.Abs(state.lastResidual); surprise > 0 {
		return surprise
	}

	_, magnitude, ok := resolvedChange(prices)

	if ok && magnitude > 0 {
		return magnitude
	}

	if len(prices) >= 2 {
		spread, spreadErr := touchSpread(prices)

		if spreadErr == nil {
			price := prices[len(prices)-1]

			if price > 0 && spread > 0 {
				return spread / price
			}
		}
	}

	return state.featureIntensityBaseline()
}

func (state *symbolState) movementConfidence(forecast float64, prices []float64) (float64, error) {
	scale := state.movementScale(prices)

	if scale <= 0 {
		scale = state.scaledResidualScale()
	}

	if scale <= 0 {
		return 0, fmt.Errorf("prediction: movement scale must be positive")
	}

	units, err := movementUnits(forecast, scale)

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

	_, magnitude := anchorChange(prices[0], prices[len(prices)-1])

	return magnitude
}

func (state *symbolState) featureIntensityBaseline() float64 {
	featureSum := 0.0
	featureCount := 0

	for _, feature := range state.features {
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
