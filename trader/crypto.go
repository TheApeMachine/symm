package trader

import (
	"context"
	"fmt"
	"math"
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
	bid       float64
	ask       float64
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
	measurements []engine.Measurement
	pairs        map[string]*pairState
	returns      map[string]*adaptive.EMA
	returnCount  map[string]int
	pulses       int
	seq          int
	quoteReady   int
	errorSum     float64
	errorCount   int
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
		measurements: make([]engine.Measurement, 0),
		pairs:        make(map[string]*pairState),
		returns:      make(map[string]*adaptive.EMA),
		returnCount:  make(map[string]int),
	}

	for _, channel := range []string{"measurements", "feedback", "tick", "executions", "exits", "wallet"} {
		crypto.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		crypto.subscribers[channel] = crypto.broadcasts[channel].Subscribe("crypto:"+channel, 128)
	}

	if errnie.Error(errnie.Require(map[string]any{
		"ctx":    ctx,
		"cancel": cancel,
		"pool":   pool,
		"wallet": wallet,
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
	crypto.broadcasts["wallet"].Send(&qpool.QValue[any]{Value: crypto.wallet})

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

		if row.Last > 0 {
			if state.lastPrice <= 0 {
				crypto.quoteReady++
			}

			state.lastPrice = row.Last
		}

		if row.Bid > 0 {
			state.bid = row.Bid
		}

		if row.Ask > 0 {
			state.ask = row.Ask
		}

		return nil
	case value := <-crypto.subscribers["executions"].Incoming:
		fill, ok := value.Value.(order.Fill)

		if !ok {
			return errnie.Error(fmt.Errorf("invalid execution fill: %v", value.Value))
		}

		crypto.applyFill(fill)

		return nil
	case value := <-crypto.subscribers["exits"].Incoming:
		exit, ok := value.Value.(map[string]any)

		if !ok {
			return errnie.Error(fmt.Errorf("invalid exit: %v", value.Value))
		}

		symbol, _ := exit["symbol"].(string)

		if symbol == "" {
			return errnie.Error(fmt.Errorf("exit missing symbol: %v", exit))
		}

		crypto.exit(symbol, exit["reason"])

		return nil
	default:
		now := time.Now()
		crypto.settleDue(now)

		perspectives := make(map[string]map[engine.PerspectiveType]engine.Perspective)
		pulseSignals := make([]map[string]any, 0, len(crypto.measurements))
		scoreTargets := make([]map[string]any, 0, len(crypto.measurements))
		bestReturn := 0.0
		var bestSymbol string
		var bestMeasurement engine.Measurement

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
					predicted := crypto.recordPrediction(perspective, measurement, now)
					symbol := measurement.Pairs[0].Wsname

					pulseSignals = append(pulseSignals, map[string]any{
						"symbol":          symbol,
						"source":          measurement.Source,
						"regime":          measurement.Regime,
						"reason":          measurement.Reason,
						"score":           measurement.Confidence,
						"expected_return": predicted,
						"type":            measurement.Type,
					})
					scoreTargets = append(scoreTargets, map[string]any{
						"symbol":          symbol,
						"regime":          measurement.Regime,
						"reason":          measurement.Reason,
						"score":           measurement.Confidence,
						"effective_score": predicted,
						"trail_pct":       config.System.DefaultTrailPct,
					})

					if predicted > bestReturn {
						bestReturn = predicted
						bestSymbol = symbol
						bestMeasurement = measurement
					}
				}
			}
		}

		openCount := crypto.openCount()

		if crypto.pulses >= config.System.MinWarmPulses &&
			openCount < config.System.MaxSlots &&
			bestReturn >= config.System.MinEdgeReturn {
			crypto.enter(bestSymbol, bestMeasurement, bestReturn)
		}

		crypto.pulses++
		crypto.seq++

		// avgError := 0.0
		// avgPrediction := 0.0

		// if crypto.errorCount > 0 {
		// 	avgError = crypto.errorSum / float64(crypto.errorCount)
		// }

		// if len(pulseSignals) > 0 {
		// 	predictionSum := 0.0

		// 	for _, signal := range pulseSignals {
		// 		if expected, ok := signal["expected_return"].(float64); ok {
		// 			predictionSum += expected
		// 		}
		// 	}

		// 	avgPrediction = predictionSum / float64(len(pulseSignals))
		// }

		crypto.publishStatus(now)

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

				crypto.errorSum += math.Abs(feedback.Error)
				crypto.errorCount++
				crypto.broadcasts["feedback"].Send(&qpool.QValue[any]{Value: feedback})
			}

			delete(state.open, source)
		}
	}
}

func (crypto *Crypto) recordPrediction(
	perspective engine.Perspective,
	measurement engine.Measurement,
	now time.Time,
) float64 {
	if len(measurement.Pairs) == 0 {
		return 0
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

	if crypto.returnCount[measurement.Source] >= config.System.MinCalibrationSamples {
		magnitude := math.Abs(sourceEMA.Value())

		if magnitude > 0 {
			predictedReturn = measurement.Confidence * magnitude
		}
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

	return predictedReturn
}

func (crypto *Crypto) enter(
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

		return
	}

	if err := crypto.wallet.ReserveEntry(slot); err != nil {
		return
	}

	fillPrice := config.System.SlippageFill(
		state.lastPrice, state.bid, state.ask, "buy", config.System.SlippageBPS, slot, nil, nil,
	)
	cost := slot
	fee := cost * crypto.wallet.FeePct / 100

	if err := crypto.wallet.SettleEntryReservation(slot, cost+fee); err != nil {
		crypto.wallet.ReleaseEntryReservation(slot)
		return
	}

	base := strings.Split(symbol, "/")[0]
	qty := (cost - fee) / fillPrice

	if qty <= 0 {
		return
	}

	crypto.wallet.Inventory[base] += qty
}

func (crypto *Crypto) exit(symbol string, reason any) {
	if crypto.wallet == nil {
		return
	}

	base := strings.Split(symbol, "/")[0]
	qty := crypto.wallet.Inventory[base]

	if qty <= 0 {
		return
	}

	state := crypto.pairs[symbol]

	if state == nil || state.lastPrice <= 0 {
		return
	}

	fillPrice := config.System.SlippageFill(
		state.lastPrice, state.bid, state.ask, "sell", config.System.SlippageBPS, qty*state.lastPrice, nil, nil,
	)
	proceeds := qty * fillPrice
	fee := proceeds * crypto.wallet.FeePct / 100

	crypto.wallet.Inventory[base] -= qty
	crypto.wallet.Balance += proceeds - fee
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
}

func (crypto *Crypto) publishStatus(now time.Time) {
	if crypto.wallet == nil {
		return
	}

	positions := make([]map[string]any, 0)
	equity := crypto.wallet.Balance + crypto.wallet.ReservedEUR

	for symbol, state := range crypto.pairs {
		if state.lastPrice <= 0 {
			continue
		}

		base := strings.Split(symbol, "/")[0]
		qty := crypto.wallet.Inventory[base]

		if qty <= 0 {
			continue
		}

		equity += qty * state.lastPrice
		positions = append(positions, map[string]any{
			"symbol": symbol,
			"qty":    qty,
			"last":   state.lastPrice,
		})
	}
}

func (crypto *Crypto) openCount() int {
	if crypto.wallet == nil {
		return 0
	}

	open := 0

	for _, qty := range crypto.wallet.Inventory {
		if qty <= 0 {
			continue
		}

		open++
	}

	return open
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
	case engine.Flow, engine.DepthFlow:
		return engine.PerspectiveFlow
	case engine.Basis, engine.LeadLag:
		return engine.PerspectiveCrossAsset
	case engine.Sentiment, engine.Causal:
		return engine.PerspectiveSentiment
	default:
		return engine.PerspectiveMicrostructure
	}
}
