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
	source          string
	sources         []string
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
		quotes:      make(map[string]lastQuote),
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

				prediction.stateMu.Lock()
				eventTime := prediction.observeTicker(row)
				prediction.settleDue(row, eventTime)
				prediction.stateMu.Unlock()
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

	if price, ok := prediction.prices[symbol]; ok && price > 0 {
		return price
	}

	return prediction.quotes[symbol].last
}

/*
LastQuote returns the cached last/bid/ask for one symbol together with the
exchange event timestamp the quote was observed at. ok is false when no
ticker has been received for that symbol yet.
*/
func (prediction *Prediction) LastQuote(symbol string) (last, bid, ask float64, at time.Time, ok bool) {
	prediction.stateMu.Lock()
	defer prediction.stateMu.Unlock()

	quote, ok := prediction.quotes[symbol]

	if !ok {
		return 0, 0, 0, time.Time{}, false
	}

	return quote.last, quote.bid, quote.ask, quote.at, true
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

func (prediction *Prediction) observeTicker(row market.TickerRow) time.Time {
	if row.Symbol == "" || row.Last <= 0 {
		return time.Time{}
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

	eventTime := ParseEventTime(row.Timestamp)
	prediction.prices[row.Symbol] = row.Last
	prediction.quotes[row.Symbol] = lastQuote{
		last:  row.Last,
		bid:   row.Bid,
		ask:   row.Ask,
		at:    eventTime,
		local: time.Now(),
	}

	return eventTime
}

/*
ParseEventTime decodes Kraken's RFC3339Nano timestamp; returns zero time when
the string is empty or malformed (callers fall back to wall clock).
*/
func ParseEventTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000000Z"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}

	return time.Time{}
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
