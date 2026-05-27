package price

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

type openPrediction struct {
	perspective     engine.Perspective
	measurement     engine.Measurement
	predictedReturn float64
	anchorPrice     float64
	direction       int
	runway          time.Duration
	dueAt           time.Time
	predictedAt     time.Time
}

type predictionSeriesKey struct {
	source string
	symbol string
}

/*
Prediction records forward return forecasts and settles them into feedback.
*/
type Prediction struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	stateMu     sync.Mutex
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	prices      map[string]float64
	open        map[string]map[string]openPrediction
	returns     map[predictionSeriesKey]*numeric.Derived
	returnSeen  map[predictionSeriesKey]bool
	marketMoves map[string]*numeric.Derived
	errorSum    float64
	errorCount  int
}

func NewPrediction(ctx context.Context, pool *qpool.Q) *Prediction {
	ctx, cancel := context.WithCancel(ctx)

	prediction := &Prediction{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		prices:      make(map[string]float64),
		open:        make(map[string]map[string]openPrediction),
		returns:     make(map[predictionSeriesKey]*numeric.Derived),
		returnSeen:  make(map[predictionSeriesKey]bool),
		marketMoves: make(map[string]*numeric.Derived),
	}

	for _, channel := range []string{"tick"} {
		prediction.subscribers[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond).
			Subscribe("prediction:"+channel, 128)
	}

	prediction.broadcasts["feedback"] = pool.CreateBroadcastGroup("feedback", 10*time.Millisecond)
	prediction.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	return prediction
}

func (prediction *Prediction) Start() error {
	return nil
}

func (prediction *Prediction) State() engine.State {
	return engine.READY
}

func (prediction *Prediction) Close() error {
	prediction.cancel()
	return nil
}

func (prediction *Prediction) Tick() error {
	for {
		select {
		case <-prediction.ctx.Done():
			return prediction.ctx.Err()
		case value := <-prediction.subscribers["tick"].Incoming:
			row, ok := value.Value.(market.TickerRow)

			if !ok {
				return errnie.Error(fmt.Errorf("invalid ticker row: %v", value.Value))
			}

			prediction.stateMu.Lock()

			prediction.observeTicker(row)
			prediction.settleDue(time.Now())
			prediction.stateMu.Unlock()
		}
	}
}

func (prediction *Prediction) SeedReturnCalibration(source, symbol string, magnitude float64) {
	prediction.stateMu.Lock()
	defer prediction.stateMu.Unlock()

	key := predictionSeriesKey{source: source, symbol: symbol}
	returns := prediction.returnSeries(key)

	if _, err := returns.Push(magnitude); err != nil {
		errnie.Error(err)
	}

	prediction.returnSeen[key] = true
}

func (prediction *Prediction) Record(
	perspective engine.Perspective,
	measurement engine.Measurement,
	anchorPrice float64,
	now time.Time,
) float64 {
	prediction.stateMu.Lock()
	defer prediction.stateMu.Unlock()

	if len(measurement.Pairs) == 0 {
		return 0
	}

	symbol := measurement.Pairs[0].Wsname
	runway := measurementRunway(measurement)
	scale := prediction.returnScale(measurement.Source, symbol)

	predictedReturn := measurement.Confidence * scale

	bySource := prediction.open[symbol]

	if bySource == nil {
		bySource = make(map[string]openPrediction)
		prediction.open[symbol] = bySource
	}

	bySource[measurement.Source] = openPrediction{
		perspective:     perspective,
		measurement:     measurement,
		predictedReturn: predictedReturn,
		anchorPrice:     anchorPrice,
		direction:       measurementDirection(measurement),
		runway:          runway,
		dueAt:           now.Add(runway),
		predictedAt:     now,
	}

	prediction.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
		"event":     "prediction",
		"source":    measurement.Source,
		"symbol":    symbol,
		"value":     predictedReturn,
		"ts":        now.UTC().Format(time.RFC3339Nano),
		"due_at":    now.Add(runway).UTC().Format(time.RFC3339Nano),
		"runway_ms": runway.Milliseconds(),
	}})

	return predictedReturn
}

func (prediction *Prediction) settleDue(now time.Time) {
	for symbol, bySource := range prediction.open {
		lastPrice := prediction.prices[symbol]

		if lastPrice <= 0 {
			continue
		}

		for source, open := range bySource {
			if now.Before(open.dueAt) {
				continue
			}

			actualReturn := 0.0

			if open.anchorPrice > 0 {
				actualReturn = float64(open.direction) *
					(lastPrice - open.anchorPrice) / open.anchorPrice

				key := predictionSeriesKey{source: open.measurement.Source, symbol: symbol}
				returns := prediction.returnSeries(key)

				if _, err := returns.Push(actualReturn); err != nil {
					errnie.Error(err)
				}

				prediction.returnSeen[key] = true
			}

			if engine.ValidPredictionFeedback(engine.PredictionFeedback{
				Source:          open.measurement.Source,
				Symbol:          symbol,
				PredictedReturn: open.predictedReturn,
				Unanchored:      open.anchorPrice <= 0,
			}) {
				feedback := engine.PredictionFeedback{
					Source:          open.measurement.Source,
					Symbol:          symbol,
					Type:            open.measurement.Type,
					Regime:          open.measurement.Regime,
					Reason:          open.measurement.Reason,
					Confidence:      open.measurement.Confidence,
					PredictedReturn: open.predictedReturn,
					ActualReturn:    actualReturn,
					Error:           open.predictedReturn - actualReturn,
					Runway:          open.runway,
					PredictedAt:     open.predictedAt,
					DueAt:           open.dueAt,
					SettledAt:       now,
					Unanchored:      open.anchorPrice <= 0,
				}

				prediction.errorSum += math.Abs(feedback.Error)
				prediction.errorCount++
				prediction.broadcasts["feedback"].Send(&qpool.QValue[any]{Value: feedback})
			}

			delete(bySource, source)
		}
	}
}

func (prediction *Prediction) RunningMeanError() float64 {
	prediction.stateMu.Lock()
	defer prediction.stateMu.Unlock()

	if prediction.errorCount == 0 {
		return 0
	}

	return prediction.errorSum / float64(prediction.errorCount)
}

func (prediction *Prediction) LastPrice(symbol string) float64 {
	prediction.stateMu.Lock()
	defer prediction.stateMu.Unlock()

	return prediction.prices[symbol]
}

func (prediction *Prediction) returnSeries(key predictionSeriesKey) *numeric.Derived {
	returns := prediction.returns[key]

	if returns == nil {
		returns = numeric.NewDerived(numeric.WithDynamics(adaptive.NewEMA(0)))
		prediction.returns[key] = returns
	}

	return returns
}

func (prediction *Prediction) marketMove(symbol string) *numeric.Derived {
	move := prediction.marketMoves[symbol]

	if move == nil {
		move = numeric.NewDerived(numeric.WithDynamics(adaptive.NewEMA(0)))
		prediction.marketMoves[symbol] = move
	}

	return move
}

func (prediction *Prediction) observeTicker(row market.TickerRow) {
	if row.Symbol == "" || row.Last <= 0 {
		return
	}

	previous := prediction.prices[row.Symbol]

	if previous > 0 {
		relativeMove := math.Abs((row.Last - previous) / previous)

		if relativeMove > 0 {
			if _, err := prediction.marketMove(row.Symbol).Push(relativeMove); err != nil {
				errnie.Error(err)
			}
		}
	}

	prediction.prices[row.Symbol] = row.Last
}

func (prediction *Prediction) returnScale(source, symbol string) float64 {
	key := predictionSeriesKey{source: source, symbol: symbol}

	if prediction.returnSeen[key] {
		return prediction.returnSeries(key).Value()
	}

	return prediction.marketMove(symbol).Value()
}

func measurementDirection(measurement engine.Measurement) int {
	if measurement.Type == engine.Dump {
		return -1
	}

	return 1
}

func measurementRunway(measurement engine.Measurement) time.Duration {
	switch measurement.Type {
	case engine.Flow, engine.DepthFlow:
		return config.System.FlowHoldBeforeExit
	case engine.Causal:
		return config.System.MinHoldBeforeRotate
	default:
		return config.System.ScalpHoldBeforeExit
	}
}
