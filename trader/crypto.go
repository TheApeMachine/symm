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
	ctx            context.Context
	cancel         context.CancelFunc
	pool           *qpool.Q
	broadcasts     map[string]*qpool.BroadcastGroup
	subscribers    map[string]*qpool.Subscriber
	ui             *qpool.BroadcastGroup
	wallet         *Wallet
	predictions    *price.Prediction
	portfolioRisk  *PortfolioRisk
	kellySizer     *KellySizer
	restingEntries map[string]restingEntry
	pulses         int
	seq            int
}

func NewCrypto(
	ctx context.Context,
	pool *qpool.Q,
	wallet *Wallet,
	predictions *price.Prediction,
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:            ctx,
		cancel:         cancel,
		pool:           pool,
		broadcasts:     make(map[string]*qpool.BroadcastGroup),
		subscribers:    make(map[string]*qpool.Subscriber),
		wallet:         wallet,
		predictions:    predictions,
		portfolioRisk:  NewPortfolioRisk(),
		kellySizer:     NewKellySizer(engine.DefaultCalibrationParams()),
		restingEntries: make(map[string]restingEntry),
	}

	crypto.subscribers["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond).
		Subscribe("crypto:measurements", 128)

	crypto.subscribers["exits"] = pool.CreateBroadcastGroup("exits", 10*time.Millisecond).
		Subscribe("crypto:exits", 128)

	crypto.subscribers["feedback"] = pool.CreateBroadcastGroup("feedback", 10*time.Millisecond).
		Subscribe("crypto:feedback", 128)

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
	case value := <-crypto.subscribers["feedback"].Incoming:
		feedback, ok := value.Value.(engine.PredictionFeedback)

		if !ok {
			return errnie.Error(fmt.Errorf("invalid prediction feedback: %v", value.Value))
		}

		crypto.kellySizer.ApplyFeedback(feedback)

		return nil
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
	case value := <-crypto.subscribers["exits"].Incoming:
		exit, ok := value.Value.(engine.Exit)

		if !ok {
			return errnie.Error(fmt.Errorf("invalid exit data: %v", value.Value))
		}

		return crypto.handleExit(exit)
	default:
		errnie.Warn("this just feels like, spinning plates, system=crypto")
		return nil
	}
}

func (crypto *Crypto) handleExit(exitSignal engine.Exit) error {
	if crypto.wallet == nil {
		return errnie.Error(fmt.Errorf("wallet is required for exit"))
	}

	if !engine.ValidExit(exitSignal) {
		return errnie.Error(fmt.Errorf("invalid exit signal: %+v", exitSignal))
	}

	symbol := exitSignal.Symbol
	reason := exitSignal.Reason

	base := strings.Split(symbol, "/")[0]
	qty := crypto.wallet.Inventory[base]

	if qty <= config.System.LiveInventoryEpsilon {
		return nil
	}

	peakExit := exitSignal.Urgency >= config.System.ExitPeakUrgency &&
		(exitSignal.Reason == engine.ExitReasonImbalanceFlip ||
			exitSignal.Reason == engine.ExitReasonPressureFade)

	if crypto.wallet.Type == PaperWallet {
		last := crypto.predictions.LastPrice(symbol)

		if last <= 0 {
			return errnie.Error(fmt.Errorf("no last price for paper exit: %s", symbol))
		}

		fillPrice := config.System.SlippageFill(
			last, last, last, "sell", config.System.SlippageBPS, qty*last, nil, nil,
		)

		if fillPrice <= 0 {
			return errnie.Error(fmt.Errorf("invalid fill price for paper exit: %s", symbol))
		}

		revenue := qty * fillPrice
		fee := revenue * crypto.wallet.FeePct / 100

		crypto.wallet.Inventory[base] = 0
		crypto.wallet.Balance += revenue - fee

		crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
			"event":   logicEvent(peakExit, "simulated_exit"),
			"symbol":  symbol,
			"qty":     qty,
			"price":   fillPrice,
			"reason":  reason,
			"urgency": exitSignal.Urgency,
		}})
		crypto.broadcasts["wallet"].Send(&qpool.QValue[any]{Value: crypto.wallet})

		return nil
	}

	if peakExit {
		last := crypto.predictions.LastPrice(symbol)

		crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
			"event":   "peak_exit",
			"symbol":  symbol,
			"qty":     qty,
			"price":   last,
			"reason":  reason,
			"urgency": exitSignal.Urgency,
		}})
	}

	crypto.pool.CreateBroadcastGroup("orders", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: order.MarketSellBase(symbol, qty, ""),
	})

	return nil
}

func logicEvent(peakExit bool, defaultEvent string) string {
	if peakExit {
		return "peak_exit"
	}

	return defaultEvent
}

func (crypto *Crypto) score(batch []engine.Measurement) error {
	now := time.Now()
	perspectives := make(map[string]map[engine.PerspectiveType]engine.Perspective)
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

	crypto.defendRestingEntries(batch)

	summary := scoreOpportunities(crypto.predictions, perspectives, now)
	opportunity := summary.Opportunity

	openCount := 0

	if crypto.wallet != nil {
		for _, qty := range crypto.wallet.Inventory {
			if qty > 0 {
				openCount++
			}
		}
	}

	observeBatch(crypto.portfolioRisk, batch)

	if crypto.wallet != nil {
		equity := crypto.wallet.MarkEquity(crypto.portfolioRisk.lastPrices)
		crypto.portfolioRisk.UpdateEquity(equity, now)
	}

	entryAllowed := false
	entryBlockReason := ""

	if crypto.pulses >= config.System.MinWarmPulses &&
		openCount < config.System.MaxSlots &&
		summary.Edge >= config.System.MinEdgeReturn &&
		len(opportunity.Measurement.Pairs) > 0 &&
		crypto.wallet != nil {
		slot := crypto.kellySizer.SlotEUR(
			crypto.wallet.Balance,
			opportunity.Measurement.Source,
			opportunity.JointConfidence,
			crypto.predictions.RunningMeanError(),
		)
		entryAllowed, entryBlockReason = crypto.portfolioRisk.AllowEntry(
			crypto.wallet,
			opportunity.Measurement,
			slot,
			openSymbols(crypto.wallet),
		)

		if entryAllowed {
			crypto.enter(opportunity, slot)
			crypto.broadcasts["wallet"].Send(&qpool.QValue[any]{Value: crypto.wallet})
		}
	}

	if entryBlockReason != "" {
		crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
			"event":  "entry_blocked",
			"symbol": opportunity.Measurement.Pairs[0].Wsname,
			"reason": entryBlockReason,
		}})
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

	if summary.PredictedCount > 0 {
		avgPrediction = summary.PredictedSum / float64(summary.PredictedCount)
	}

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":            "engine_pulse",
		"ts":               now.UTC().Format(time.RFC3339Nano),
		"seq":              crypto.seq,
		"measurements":     len(batch),
		"open":             openCount,
		"avg_prediction":   avgPrediction,
		"avg_error":        crypto.predictions.RunningMeanError(),
		"forecast_symbols": summary.PredictedCount,
		"entry_blocked":    entryBlockReason,
		"joint_confidence": opportunity.JointConfidence,
		"fused_edge":       summary.Edge,
		"fused_sources":    opportunity.SourceCount,
	}})

	if crypto.wallet != nil {
		crypto.broadcasts["wallet"].Send(&qpool.QValue[any]{Value: crypto.wallet})
	}

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
