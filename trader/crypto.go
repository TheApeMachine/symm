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
	"github.com/theapemachine/symm/kraken/order"
	"github.com/theapemachine/symm/price"
)

/*
Crypto combines measurements into perspectives, records predictions, and enters trades.
*/
type Crypto struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	ui          *qpool.BroadcastGroup
	wallet      *Wallet
	predictions *price.Prediction
	pulses      int
	seq         int
}

func NewCrypto(
	ctx context.Context,
	pool *qpool.Q,
	wallet *Wallet,
	predictions *price.Prediction,
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		wallet:      wallet,
		predictions: predictions,
	}

	crypto.subscribers["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond).
		Subscribe("crypto:measurements", 128)

	crypto.broadcasts["confidence"] = pool.CreateBroadcastGroup("confidence", 10*time.Millisecond)
	crypto.broadcasts["wallet"] = pool.CreateBroadcastGroup("wallet", 10*time.Millisecond)
	crypto.ui = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	if errnie.Error(errnie.Require(map[string]any{
		"ctx":         ctx,
		"cancel":      cancel,
		"pool":        pool,
		"wallet":      wallet,
		"predictions": predictions,
	})) != nil {
		return nil
	}

	return crypto
}

func (crypto *Crypto) Start() error {
	crypto.broadcasts["wallet"].Send(&qpool.QValue[any]{Value: crypto.wallet})
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
	case value := <-crypto.subscribers["measurements"].Incoming:
		measurement, ok := value.Value.(engine.Measurement)

		if !ok {
			return errnie.Error(fmt.Errorf("invalid measurement: %v", value.Value))
		}

		batch := []engine.Measurement{measurement}

		for {
			select {
			case next := <-crypto.subscribers["measurements"].Incoming:
				payload, ok := next.Value.(engine.Measurement)

				if !ok {
					return errnie.Error(fmt.Errorf("invalid measurement: %v", next.Value))
				}

				batch = append(batch, payload)
			default:
				return crypto.score(batch)
			}
		}
	default:
		return nil
	}
}

func (crypto *Crypto) score(batch []engine.Measurement) error {
	now := time.Now()
	perspectives := make(map[string]map[engine.PerspectiveType]engine.Perspective)
	bestReturn := 0.0
	bestMeasurement := engine.Measurement{}
	predictedSum := 0.0
	predictedCount := 0
	confidenceSums := make(map[string]float64)
	confidenceCounts := make(map[string]int)

	for _, measurement := range batch {
		if len(measurement.Pairs) == 0 {
			continue
		}

		if measurement.Source != "" {
			confidenceSums[measurement.Source] += measurement.Confidence
			confidenceCounts[measurement.Source]++
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
				predicted := crypto.predictions.Record(
					perspective,
					measurement,
					anchorPrice(measurement),
					now,
				)

				if predicted > 0 {
					predictedSum += predicted
					predictedCount++
				}

				if predicted > bestReturn {
					bestReturn = predicted
					bestMeasurement = measurement
				}
			}
		}
	}

	openCount := 0

	if crypto.wallet != nil {
		for _, qty := range crypto.wallet.Inventory {
			if qty > 0 {
				openCount++
			}
		}
	}

	if crypto.pulses >= config.System.MinWarmPulses &&
		openCount < config.System.MaxSlots &&
		bestReturn >= config.System.MinEdgeReturn &&
		len(bestMeasurement.Pairs) > 0 {
		crypto.enter(bestMeasurement)
		crypto.broadcasts["wallet"].Send(&qpool.QValue[any]{Value: crypto.wallet})
	}

	for source, sum := range confidenceSums {
		count := confidenceCounts[source]

		if count == 0 {
			continue
		}

		crypto.broadcasts["confidence"].Send(&qpool.QValue[any]{Value: map[string]any{
			"source":     source,
			"confidence": sum / float64(count),
			"count":      count,
		}})
	}

	avgPrediction := 0.0

	if predictedCount > 0 {
		avgPrediction = predictedSum / float64(predictedCount)
	}

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":            "engine_pulse",
		"ts":               now.UTC().Format(time.RFC3339Nano),
		"seq":              crypto.seq,
		"measurements":     len(batch),
		"open":             openCount,
		"avg_prediction":   avgPrediction,
		"avg_error":        crypto.predictions.RunningMeanError(),
		"forecast_symbols": predictedCount,
	}})

	crypto.pulses++
	crypto.seq++

	return nil
}

func (crypto *Crypto) Close() error {
	crypto.cancel()
	return nil
}

func anchorPrice(measurement engine.Measurement) float64 {
	if measurement.Last > 0 {
		return measurement.Last
	}

	if measurement.Bid > 0 && measurement.Ask > 0 {
		return (measurement.Bid + measurement.Ask) / 2
	}

	return 0
}

func (crypto *Crypto) enter(measurement engine.Measurement) {
	if crypto.wallet == nil || len(measurement.Pairs) == 0 {
		return
	}

	symbol := measurement.Pairs[0].Wsname
	last := anchorPrice(measurement)
	bid := measurement.Bid
	ask := measurement.Ask

	if last <= 0 {
		return
	}

	if bid <= 0 {
		bid = last
	}

	if ask <= 0 {
		ask = last
	}

	slot := crypto.wallet.Balance * config.System.MaxSlotPct / 100

	if slot < config.System.MinCostEUR {
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
		last, bid, ask, "buy", config.System.SlippageBPS, slot, nil, nil,
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
