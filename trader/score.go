package trader

import (
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/numeric/adaptive"
)

func (crypto *Crypto) tryScorePendingMeasurements() bool {
	select {
	case value := <-crypto.subscribers["measurements"].Incoming:
		if err := crypto.ingestMeasurement(value.Value); err != nil {
			errnie.Error(err)
		}

		return true
	default:
		return false
	}
}

func (crypto *Crypto) ingestMeasurement(raw any) error {
	measurement, ok := raw.(engine.Measurement)

	if !ok {
		return fmt.Errorf("invalid measurement: %v", raw)
	}

	return crypto.scoreBatch(measurement)
}

func (crypto *Crypto) scoreBatch(first engine.Measurement) error {
	batch := []engine.Measurement{first}

	for {
		select {
		case next := <-crypto.subscribers["measurements"].Incoming:
			payload, ok := next.Value.(engine.Measurement)

			if !ok {
				return fmt.Errorf("invalid measurement: %v", next.Value)
			}

			batch = append(batch, payload)
		default:
			return crypto.score(batch)
		}
	}
}

func (crypto *Crypto) score(batch []engine.Measurement) error {
	now := time.Now()
	perspectives := make(map[string]map[engine.PerspectiveType]engine.Perspective)

	crypto.observeSourceConfidence(batch)
	crypto.buildPerspectives(batch, perspectives)
	crypto.defendRestingEntries(batch)

	summary := scoreOpportunities(crypto.predictions, perspectives, now)
	opportunity := summary.Opportunity
	openCount := crypto.openCount()

	observeBatch(crypto.portfolioRisk, batch, now)
	crypto.observeOpenPrices(batch, now)
	crypto.updateEquity(now)

	entryBlockReason := crypto.tryEnterOpportunity(summary, opportunity)

	crypto.chaseRestingEntries(batch)
	crypto.fillRestingEntries(batch)
	crypto.publishEntryBlock(entryBlockReason, opportunity)
	crypto.publishConfidence()
	crypto.publishPulse(now, batch, summary, opportunity, openCount, entryBlockReason)

	if crypto.wallet != nil {
		crypto.sendWallet()
	}

	crypto.pulses++
	crypto.seq++

	return nil
}

func (crypto *Crypto) observeSourceConfidence(batch []engine.Measurement) {
	for _, measurement := range batch {
		if measurement.Source == "" {
			continue
		}

		ema := crypto.sourceConfidence[measurement.Source]

		if ema == nil {
			ema = adaptive.NewEMA(0)
			crypto.sourceConfidence[measurement.Source] = ema
		}

		if _, err := ema.Next(0, measurement.Confidence); err != nil {
			errnie.Error(err)
		}
	}
}

func (crypto *Crypto) buildPerspectives(
	batch []engine.Measurement,
	perspectives map[string]map[engine.PerspectiveType]engine.Perspective,
) {
	for _, measurement := range batch {
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
}

func (crypto *Crypto) tryEnterOpportunity(
	summary scoreSummary,
	opportunity tradeOpportunity,
) string {
	if summary.Edge <= 0 || len(opportunity.Measurement.Pairs) == 0 || crypto.wallet == nil {
		return ""
	}

	slot := crypto.kellySizer.SlotEUR(
		crypto.wallet.Balance,
		engine.PerspectiveSource(opportunity.PerspectiveType),
		opportunity.Regime,
		opportunity.JointConfidence,
		crypto.predictions.RunningMeanError(),
	)
	entryAllowed, entryBlockReason := crypto.portfolioRisk.AllowEntry(
		crypto.wallet,
		opportunity.Measurement,
		slot,
		activeSymbols(crypto.wallet, crypto.restingEntries),
	)

	if !entryAllowed {
		return entryBlockReason
	}

	crypto.enter(opportunity, slot)
	crypto.sendWallet()

	return ""
}

func (crypto *Crypto) publishEntryBlock(
	entryBlockReason string,
	opportunity tradeOpportunity,
) {
	if entryBlockReason == "" {
		return
	}

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":  "entry_blocked",
		"symbol": opportunity.Measurement.Pairs[0].Wsname,
		"reason": entryBlockReason,
	}})
}

func (crypto *Crypto) publishConfidence() {
	for source, ema := range crypto.sourceConfidence {
		confidence := ema.Value()

		crypto.broadcasts["confidence"].Send(&qpool.QValue[any]{Value: map[string]any{
			"source":     source,
			"confidence": confidence,
		}})
	}
}

func (crypto *Crypto) publishPulse(
	now time.Time,
	batch []engine.Measurement,
	summary scoreSummary,
	opportunity tradeOpportunity,
	openCount int,
	entryBlockReason string,
) {
	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":            "engine_pulse",
		"ts":               now.UTC().Format(time.RFC3339Nano),
		"seq":              crypto.seq,
		"measurements":     len(batch),
		"open":             openCount,
		"avg_prediction":   averagePrediction(summary),
		"avg_error":        crypto.predictions.RunningMeanError(),
		"forecast_symbols": summary.PredictedCount,
		"entry_blocked":    entryBlockReason,
		"joint_confidence": pulseConfidence(batch, opportunity),
		"batch_confidence": batchConfidence(batch),
		"fused_edge":       summary.Edge,
		"fused_sources":    opportunity.SourceCount,
		"entry_friction":   opportunity.Friction,
	}})
}

func averagePrediction(summary scoreSummary) float64 {
	if summary.PredictedCount == 0 {
		return 0
	}

	return summary.PredictedSum / float64(summary.PredictedCount)
}

func pulseConfidence(batch []engine.Measurement, opportunity tradeOpportunity) float64 {
	if opportunity.JointConfidence > 0 {
		return opportunity.JointConfidence
	}

	return batchConfidence(batch)
}

func batchConfidence(batch []engine.Measurement) float64 {
	confidence := 0.0

	for _, measurement := range batch {
		if measurement.Confidence > confidence {
			confidence = measurement.Confidence
		}
	}

	return confidence
}
