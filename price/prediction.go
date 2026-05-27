package price

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/market"
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
	returns     map[string]*adaptive.EMA
	returnCount map[string]int
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
		returns:     make(map[string]*adaptive.EMA),
		returnCount: make(map[string]int),
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
	errnie.Info("starting prediction tick")

	for {
		prediction.stateMu.Lock()
		defer prediction.stateMu.Unlock()

		select {
		case <-prediction.ctx.Done():
			return prediction.ctx.Err()
		case value := <-prediction.subscribers["tick"].Incoming:
			row := value.Value.(market.TickerRow)

			if row.Last > 0 {
				prediction.prices[row.Symbol] = row.Last
			}

			prediction.settleDue(time.Now())
		}
	}
}

func (prediction *Prediction) SeedReturnCalibration(source string, magnitude float64) {
	prediction.stateMu.Lock()
	defer prediction.stateMu.Unlock()

	returnEMA := prediction.returnEMA(source)
	_, _ = returnEMA.Next(0, magnitude)
	prediction.returnCount[source] = config.System.MinCalibrationSamples
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
	scale := math.Abs(prediction.returnEMA(measurement.Source).Value())

	if scale <= 0 {
		scale = 1.0
	}

	predictedReturn := measurement.Confidence * scale

	if predictedReturn <= 0 {
		predictedReturn = 0.01 * scale
	}

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

			actualReturn := float64(open.direction) *
				(lastPrice - open.anchorPrice) / open.anchorPrice

			if open.anchorPrice > 0 {
				returnEMA := prediction.returnEMA(open.measurement.Source)
				_, _ = returnEMA.Next(0, actualReturn)
				prediction.returnCount[open.measurement.Source]++
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

func (prediction *Prediction) returnEMA(source string) *adaptive.EMA {
	ema := prediction.returns[source]

	if ema == nil {
		ema = adaptive.NewEMA(0)
		prediction.returns[source] = ema
	}

	return ema
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
