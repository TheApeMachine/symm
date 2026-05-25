package trader

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/order"
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

type pairState struct {
	lastPrice float64
	open      map[string]openPrediction
}

/*
Crypto combines measurements into perspectives, records predictions, and settles feedback.
*/
type Crypto struct {
	ctx          context.Context
	cancel       context.CancelFunc
	pool         *qpool.Q
	broadcasts   map[string]*qpool.BroadcastGroup
	subscribers  map[string]*qpool.Subscriber
	wallet       *Wallet
	portfolio    *Portfolio
	measurements []engine.Measurement
	pairs        map[string]*pairState
	returns      map[string]*adaptive.EMA
	returnCount  map[string]int
}

func NewCrypto(
	ctx context.Context,
	pool *qpool.Q,
	wallet *Wallet,
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		broadcasts:   make(map[string]*qpool.BroadcastGroup),
		subscribers:  make(map[string]*qpool.Subscriber),
		wallet:       wallet,
		portfolio:    NewPortfolio(wallet),
		measurements: make([]engine.Measurement, 0),
		pairs:        make(map[string]*pairState),
		returns:      make(map[string]*adaptive.EMA),
		returnCount:  make(map[string]int),
	}

	crypto.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	crypto.subscribers["measurements"] = crypto.broadcasts["measurements"].Subscribe("crypto:measurements", 128)
	crypto.broadcasts["feedback"] = pool.CreateBroadcastGroup("feedback", 10*time.Millisecond)
	crypto.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	tick := pool.CreateBroadcastGroup("tick", 10*time.Millisecond)
	crypto.subscribers["tick"] = tick.Subscribe("crypto:tick", 128)

	executions := pool.CreateBroadcastGroup("executions", 10*time.Millisecond)
	crypto.subscribers["executions"] = executions.Subscribe("crypto:executions", 128)

	if errnie.Error(errnie.Require(map[string]any{
		"ctx":       ctx,
		"cancel":    cancel,
		"pool":      pool,
		"wallet":    wallet,
		"portfolio": crypto.portfolio,
	})) != nil {
		return nil
	}

	return crypto
}

func (crypto *Crypto) Start() error {
	return nil
}

func (crypto *Crypto) State() engine.State {
	return engine.READY
}

func (crypto *Crypto) Tick() error {
	select {
	case <-crypto.ctx.Done():
		crypto.cancel()
		return crypto.ctx.Err()
	case measurement := <-crypto.subscribers["measurements"].Incoming:
		payload, ok := measurement.Value.(engine.Measurement)

		if !ok {
			return errnie.Error(fmt.Errorf("invalid measurement: %v", measurement.Value))
		}

		crypto.measurements = append(crypto.measurements, payload)

		return nil
	case value := <-crypto.subscribers["tick"].Incoming:
		row := value.Value.(market.TickerRow)
		state := crypto.pairs[row.Symbol]

		if state == nil {
			state = &pairState{open: make(map[string]openPrediction)}
			crypto.pairs[row.Symbol] = state
		}

		state.lastPrice = row.Last

		return nil
	case value := <-crypto.subscribers["executions"].Incoming:
		fill, ok := value.Value.(order.Fill)

		if !ok {
			return errnie.Error(fmt.Errorf("invalid execution fill: %v", value.Value))
		}

		crypto.applyFill(fill)

		return nil
	default:
		now := time.Now()
		crypto.settleDue(now)

		perspectives := make(map[string]map[engine.PerspectiveType]engine.Perspective)

		for _, measurement := range crypto.measurements {
			if len(measurement.Pairs) == 0 {
				continue
			}

			symbol := measurement.Pairs[0].Wsname
			perspectiveType := perspectiveType(measurement)
			byType := perspectives[symbol]

			if byType == nil {
				byType = make(map[engine.PerspectiveType]engine.Perspective)
				perspectives[symbol] = byType
			}

			perspective := byType[perspectiveType]
			perspective.Type = perspectiveType
			perspective.Measurements = append(perspective.Measurements, measurement)
			byType[perspectiveType] = perspective
		}

		for _, byType := range perspectives {
			for _, perspective := range byType {
				for _, measurement := range perspective.Measurements {
					crypto.recordPrediction(perspective, measurement, now)
				}
			}
		}

		crypto.measurements = crypto.measurements[:0]

		return nil
	}
}

func (crypto *Crypto) Close() error {
	crypto.cancel()
	return nil
}

func (crypto *Crypto) settleDue(now time.Time) {
	for symbol, state := range crypto.pairs {
		if state.lastPrice <= 0 {
			continue
		}

		for source, prediction := range state.open {
			if now.Before(prediction.dueAt) {
				continue
			}

			actualReturn := float64(prediction.direction) *
				(state.lastPrice - prediction.anchorPrice) / prediction.anchorPrice

			if prediction.anchorPrice > 0 {
				returnEMA := crypto.returnEMA(prediction.measurement.Source)
				_, _ = returnEMA.Next(0, actualReturn)
				crypto.returnCount[prediction.measurement.Source]++
			}

			if engine.ValidPredictionFeedback(engine.PredictionFeedback{
				Source:          prediction.measurement.Source,
				Symbol:          symbol,
				PredictedReturn: prediction.predictedReturn,
				Unanchored:      prediction.anchorPrice <= 0,
			}) {
				feedback := engine.PredictionFeedback{
					Source:          prediction.measurement.Source,
					Symbol:          symbol,
					Type:            prediction.measurement.Type,
					Regime:          prediction.measurement.Regime,
					Reason:          prediction.measurement.Reason,
					Confidence:      prediction.measurement.Confidence,
					PredictedReturn: prediction.predictedReturn,
					ActualReturn:    actualReturn,
					Error:           prediction.predictedReturn - actualReturn,
					Runway:          prediction.runway,
					SettledAt:       now,
					Unanchored:      prediction.anchorPrice <= 0,
				}

				crypto.broadcasts["feedback"].Send(&qpool.QValue[any]{Value: feedback})
				crypto.sendUI(map[string]any{
					"event":            "decision_trace",
					"phase":            "settle",
					"symbol":           symbol,
					"source":           source,
					"predicted_return": prediction.predictedReturn,
					"actual_return":    actualReturn,
					"perspective":      prediction.perspective.Type,
				})
			}

			delete(state.open, source)
		}
	}
}

func (crypto *Crypto) recordPrediction(
	perspective engine.Perspective,
	measurement engine.Measurement,
	now time.Time,
) {
	if len(measurement.Pairs) == 0 {
		return
	}

	symbol := measurement.Pairs[0].Wsname
	state := crypto.pairs[symbol]

	if state == nil {
		state = &pairState{open: make(map[string]openPrediction)}
		crypto.pairs[symbol] = state
	}

	runway := measurementRunway(measurement)
	predictedReturn := 0.0
	sourceEMA := crypto.returnEMA(measurement.Source)

	if crypto.returnCount[measurement.Source] >= config.System.MinCalibrationSamples &&
		sourceEMA.Value() > 0 {
		predictedReturn = measurement.Confidence * sourceEMA.Value()
	}

	state.open[measurement.Source] = openPrediction{
		perspective:     perspective,
		measurement:     measurement,
		predictedReturn: predictedReturn,
		anchorPrice:     state.lastPrice,
		direction:       measurementDirection(measurement),
		runway:          runway,
		dueAt:           now.Add(runway),
		predictedAt:     now,
	}

	crypto.sendUI(map[string]any{
		"event":            "decision_trace",
		"phase":            "predict",
		"symbol":           symbol,
		"source":           measurement.Source,
		"confidence":       measurement.Confidence,
		"predicted_return": predictedReturn,
		"perspective":      perspective.Type,
		"runway_ms":        runway.Milliseconds(),
	})

	if predictedReturn >= config.System.MinEdgeReturn {
		crypto.maybeEnter(symbol, measurement, predictedReturn)
	}
}

func (crypto *Crypto) maybeEnter(
	symbol string,
	measurement engine.Measurement,
	predictedReturn float64,
) {
	if crypto.wallet == nil {
		return
	}

	slot := crypto.wallet.Balance * config.System.MaxSlotPct / 100

	if slot < config.System.MinCostEUR {
		return
	}

	state := crypto.pairs[symbol]

	if state == nil || state.lastPrice <= 0 {
		return
	}

	if crypto.wallet.Type == CryptoWallet {
		if err := crypto.wallet.ReserveEntry(slot); err != nil {
			return
		}

		crypto.pool.CreateBroadcastGroup("orders", 10*time.Millisecond).Send(&qpool.QValue[any]{
			Value: order.MarketBuyCash(symbol, slot, 0, 0, ""),
		})

		crypto.sendUI(map[string]any{
			"event":            "decision_trace",
			"phase":            "enter",
			"symbol":           symbol,
			"source":           measurement.Source,
			"notional_eur":     slot,
			"predicted_return": predictedReturn,
			"live":             true,
		})

		return
	}

	if err := crypto.wallet.ReserveEntry(slot); err != nil {
		return
	}

	if err := crypto.wallet.SettleEntryReservation(slot, slot); err != nil {
		crypto.wallet.ReleaseEntryReservation(slot)
		return
	}

	base := strings.Split(symbol, "/")[0]
	fee := slot * crypto.wallet.FeePct / 100
	qty := (slot - fee) / state.lastPrice

	if qty <= 0 {
		return
	}

	crypto.wallet.Inventory[base] += qty

	crypto.sendUI(map[string]any{
		"event":            "decision_trace",
		"phase":            "enter",
		"symbol":           symbol,
		"source":           measurement.Source,
		"notional_eur":     slot,
		"qty":              qty,
		"predicted_return": predictedReturn,
	})
}

func (crypto *Crypto) applyFill(fill order.Fill) {
	if crypto.wallet == nil {
		return
	}

	base := strings.Split(fill.Symbol, "/")[0]
	cost := fill.Qty * fill.Price
	fee := cost * crypto.wallet.FeePct / 100

	if fill.Side == "buy" {
		reserved := crypto.wallet.ReservedEUR

		if reserved <= 0 {
			reserved = cost + fee
		}

		if err := crypto.wallet.SettleEntryReservation(reserved, cost+fee); err != nil {
			errnie.Error(err)
			return
		}

		crypto.wallet.Inventory[base] += fill.Qty
	}

	if fill.Side == "sell" {
		crypto.wallet.Inventory[base] -= fill.Qty
		crypto.wallet.Balance += cost - fee
	}

	crypto.sendUI(map[string]any{
		"event":   "decision_trace",
		"phase":   "fill",
		"symbol":  fill.Symbol,
		"side":    fill.Side,
		"qty":     fill.Qty,
		"price":   fill.Price,
		"orderID": fill.OrderID,
	})
}

func (crypto *Crypto) sendUI(payload map[string]any) {
	crypto.broadcasts["ui"].Send(&qpool.QValue[any]{
		Value: payload,
	})
}

func (crypto *Crypto) returnEMA(source string) *adaptive.EMA {
	ema := crypto.returns[source]

	if ema == nil {
		ema = adaptive.NewEMA(0)
		crypto.returns[source] = ema
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

func perspectiveType(measurement engine.Measurement) engine.PerspectiveType {
	switch measurement.Type {
	case engine.Basis, engine.LeadLag:
		return engine.PerspectiveCrossAsset
	case engine.Sentiment:
		return engine.PerspectiveSentiment
	default:
		return engine.PerspectiveMicrostructure
	}
}
