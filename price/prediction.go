package price

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric"
)

type openPrediction struct {
	perspective     engine.Perspective
	measurement     engine.Measurement
	source          string
	sources         []string
	regime          string
	predictedReturn float64
	confidence      float64
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

type stopOrder struct {
	price     float64 // hard floor; fire if price <= this
	trail     bool
	trailFrac float64 // retrace from peak that fires a trailing stop
	peak      float64
	fired     bool
}

/*
lastQuote captures the most recent ticker observation: last/bid/ask together
with the exchange event time so consumers can reject stale snapshots and so
prediction settlement uses the actual price at or after the due time rather
than whatever the cache currently holds.
*/
type lastQuote struct {
	last  float64
	bid   float64
	ask   float64
	at    time.Time
	local time.Time
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
	// prices is the cached "last price seen" per symbol. quotes carries the
	// richer (bid, ask, event-time) record introduced for trader/exit.go
	// pricing; prices is kept as a flat scalar map because the test seam
	// directly seeds it (prediction.prices["X"] = 50000). observeTicker
	// keeps the two in sync.
	prices      map[string]float64
	quotes      map[string]lastQuote
	open        map[string]map[string]openPrediction
	returns     map[predictionSeriesKey]*numeric.Derived
	returnSeen  map[predictionSeriesKey]bool
	returnModel *ReturnModel
	marketMoves map[string]*numeric.Derived
	stops       map[string]stopOrder
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
		quotes:      make(map[string]lastQuote),
		open:        make(map[string]map[string]openPrediction),
		returns:     make(map[predictionSeriesKey]*numeric.Derived),
		returnSeen:  make(map[predictionSeriesKey]bool),
		returnModel: NewReturnModel(),
		marketMoves: make(map[string]*numeric.Derived),
		stops:       make(map[string]stopOrder),
	}

	for _, channel := range []string{"tick"} {
		prediction.subscribers[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond).
			Subscribe("prediction:"+channel, 128)
	}

	prediction.broadcasts["feedback"] = pool.CreateBroadcastGroup("feedback", 10*time.Millisecond)
	prediction.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
	prediction.broadcasts["exits"] = pool.CreateBroadcastGroup("exits", 10*time.Millisecond)

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
	var wg sync.WaitGroup

	wg.Go(func() {
		for {
			select {
			case <-prediction.ctx.Done():
				return
			case value, ok := <-prediction.subscribers["tick"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("prediction tick channel closed"))
					return
				}

				row, ok := value.Value.(market.TickerRow)

				if !ok {
					errnie.Error(fmt.Errorf("invalid ticker row: %v", value.Value))
					return
				}

				eventTime := prediction.observeTicker(row)

				prediction.stateMu.Lock()
				prediction.settleDue(row, eventTime)
				stopExit, fired := prediction.checkStopLocked(row.Symbol, row.Last)
				prediction.stateMu.Unlock()

				if fired {
					prediction.broadcasts["exits"].Send(&qpool.QValue[any]{Value: stopExit})
				}
			}
		}
	})

	wg.Wait()
	return prediction.ctx.Err()
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

/*
settleDueAt settles every open prediction whose dueAt is before now using
the cached "last price" for each symbol. Retained as a test seam: live
settlement runs through settleDue(row, eventTime) so the actual fill
price at or after the due time is used.
*/
func (prediction *Prediction) settleDueAt(now time.Time) {
	for symbol := range prediction.open {
		price := prediction.prices[symbol]

		if price <= 0 {
			price = prediction.quotes[symbol].last
		}

		if price <= 0 {
			continue
		}

		row := market.TickerRow{Symbol: symbol, Last: price}
		prediction.settleDue(row, now)
	}
}

/*
settleDue is the per-tick settlement path. Callers in test code that want
to settle all open predictions at a single wall-clock moment should call
settleDueAt instead.
*/
func (prediction *Prediction) settleDue(row market.TickerRow, eventTime time.Time) {
	settlePrice := row.Last
	settledAt := eventTime

	if settledAt.IsZero() {
		settledAt = time.Now()
	}

	for symbol, bySource := range prediction.open {
		// Only settle predictions whose symbol matches the tick that just
		// arrived. Using a stale cached last for another symbol would let
		// drift accumulated since that symbol's last tick bleed into the
		// realized return label.
		if symbol != row.Symbol {
			continue
		}

		if settlePrice <= 0 {
			continue
		}

		for source, open := range bySource {
			if settledAt.Before(open.dueAt) {
				continue
			}

			actualReturn := 0.0

			if open.anchorPrice > 0 {
				actualReturn = float64(open.direction) *
					(settlePrice - open.anchorPrice) / open.anchorPrice

				prediction.returnModel.Observe(
					open.source, open.regime, open.confidence, actualReturn,
				)

				key := predictionSeriesKey{source: open.source, symbol: symbol}
				returns := prediction.returnSeries(key)

				if _, err := returns.Push(actualReturn); err != nil {
					errnie.Error(err)
				}

				prediction.returnSeen[key] = true
			}

			if engine.ValidPredictionFeedback(engine.PredictionFeedback{
				Source:          open.source,
				Symbol:          symbol,
				PredictedReturn: open.predictedReturn,
				Unanchored:      open.anchorPrice <= 0,
			}) {
				feedback := engine.PredictionFeedback{
					Source:          open.source,
					Sources:         append([]string(nil), open.sources...),
					Symbol:          symbol,
					Type:            open.measurement.Type,
					PerspectiveType: open.perspective.Type,
					Regime:          engine.FeedbackRegime(open.perspective, open.measurement),
					Reason:          open.measurement.Reason,
					Confidence:      open.confidence,
					PredictedReturn: open.predictedReturn,
					ActualReturn:    actualReturn,
					Error:           open.predictedReturn - actualReturn,
					Runway:          open.runway,
					PredictedAt:     open.predictedAt,
					DueAt:           open.dueAt,
					SettledAt:       settledAt,
					Unanchored:      open.anchorPrice <= 0,
				}

				prediction.errorSum += math.Abs(feedback.Error)
				prediction.errorCount++
				prediction.broadcasts["feedback"].Send(&qpool.QValue[any]{Value: feedback})

				// Mirror the settlement to the UI so the prediction chart can
				// place the realised return next to the forecast at the same
				// time-axis position. Predictions live in time, not cycles.
				prediction.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
					"event":            "prediction_settled",
					"ts":               settledAt.UTC().Format(time.RFC3339Nano),
					"predicted_at":     open.predictedAt.UTC().Format(time.RFC3339Nano),
					"due_at":           open.dueAt.UTC().Format(time.RFC3339Nano),
					"symbol":           symbol,
					"source":           open.source,
					"predicted_return": open.predictedReturn,
					"actual_return":    actualReturn,
					"error":            feedback.Error,
				}})
			}

			delete(bySource, source)
		}
	}
}
