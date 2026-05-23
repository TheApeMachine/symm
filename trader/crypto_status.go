package trader

import (
	"time"

	"github.com/theapemachine/symm/engine"
)

func (crypto *Crypto) publishStatus() {
	if crypto.publisher == nil {
		return
	}

	crypto.publisher.Emit(crypto.statusEvent())
}

func (crypto *Crypto) statusEvent() map[string]any {
	equity, positions := crypto.markToMarket()
	winRate := 0.0

	if crypto.tradeCount > 0 {
		winRate = float64(crypto.winCount) / float64(crypto.tradeCount)
	}

	return map[string]any{
		"event":          "status",
		"ts":             time.Now().UTC().Format(time.RFC3339Nano),
		"equity_eur":     equity,
		"cash_eur":       crypto.wallet.Balance,
		"closed_pnl_eur": crypto.closedPnL,
		"trade_count":    crypto.tradeCount,
		"win_rate":       winRate,
		"open_count":     len(crypto.holds),
		"positions":      positions,
	}
}

func (crypto *Crypto) markToMarket() (float64, []map[string]any) {
	markValue := 0.0
	positions := make([]map[string]any, 0, len(crypto.holds))

	for symbol, hold := range crypto.holds {
		last, _, _, _, ok := crypto.quote(symbol)
		if !ok || last <= 0 || hold.entryPrice <= 0 {
			last = hold.entryPrice
		}

		positionValue := hold.notional * (last / hold.entryPrice)
		markValue += positionValue

		positions = append(positions, map[string]any{
			"symbol":       symbol,
			"regime":       hold.regime,
			"entry_price":  hold.entryPrice,
			"stop_price":   hold.stopPrice,
			"peak_price":   hold.peakPrice,
			"last_price":   last,
			"trail_pct":    hold.trailPct,
			"notional_eur": hold.notional,
			"opened_at":    hold.enteredAt.UTC().Format(time.RFC3339Nano),
		})
	}

	return crypto.wallet.Balance + markValue, positions
}

func (crypto *Crypto) publishEnginePulse(
	batch []engine.Measurement,
	candidates []tradeCandidate,
) {
	if crypto.publisher == nil {
		return
	}

	signalRows := make([]map[string]any, 0, len(batch))

	for _, measurement := range batch {
		if measurement.Err != nil || len(measurement.Pairs) == 0 {
			continue
		}

		symbol := pairSymbol(measurement.Pairs[0])
		if symbol == "" {
			continue
		}

		signalRows = append(signalRows, map[string]any{
			"symbol": symbol,
			"source": measurement.Source,
			"regime": measurement.Regime,
			"reason": measurement.Reason,
			"score":  measurement.Confidence,
			"type":   regimeForType(measurement.Type),
		})
	}

	payload := map[string]any{
		"event":        "engine_pulse",
		"seq":          crypto.pulseSeq.Add(1),
		"phase":        crypto.enginePhase(),
		"measurements": len(batch),
		"candidates":   len(candidates),
		"open":         len(crypto.holds),
		"signals":      signalRows,
	}

	if crypto.engineStats != nil {
		payload["ticker_ready"] = crypto.engineStats.TickerReadyCount()
		payload["symbols_total"] = crypto.engineStats.SymbolTotal()
		payload["fluid_sampled"] = crypto.engineStats.FluidSampledCount()
		payload["fluid_warming"] = crypto.engineStats.FluidWarmingCount()
	}

	crypto.publisher.Emit(payload)
}

func (crypto *Crypto) enginePhase() string {
	if crypto.readyForTrading() {
		return "scan"
	}

	return "warming"
}

func (crypto *Crypto) publishScoreboard(
	batch []engine.Measurement,
	candidates []tradeCandidate,
	line entryLine,
) {
	if crypto.publisher == nil {
		return
	}

	targets := scoreboardFromCandidates(candidates)

	if len(targets) == 0 {
		targets = scoreboardFromBatch(batch)
	}

	crypto.publisher.Emit(map[string]any{
		"event":   "scoreboard",
		"ts":      time.Now().UTC().Format(time.RFC3339Nano),
		"line":    line.line,
		"median":  line.median,
		"mad":     line.mad,
		"targets": targets,
	})
}

func scoreboardFromCandidates(candidates []tradeCandidate) []map[string]any {
	targets := make([]map[string]any, 0, len(candidates))

	for _, candidate := range candidates {
		targets = append(targets, map[string]any{
			"symbol":          candidate.symbol,
			"regime":          candidate.regime,
			"reason":          candidate.reason,
			"score":           candidate.confidence,
			"effective_score": candidate.confidence,
			"trail_pct":       0,
			"support":         candidate.support,
		})
	}

	return targets
}

func scoreboardFromBatch(batch []engine.Measurement) []map[string]any {
	targets := make([]map[string]any, 0, len(batch))

	for _, measurement := range batch {
		if measurement.Err != nil || measurement.Confidence <= 0 || len(measurement.Pairs) == 0 {
			continue
		}

		symbol := pairSymbol(measurement.Pairs[0])
		if symbol == "" {
			continue
		}

		regime := measurement.Regime
		if regime == "" {
			regime = regimeForType(measurement.Type)
		}

		reason := measurement.Reason
		if reason == "" {
			reason = "ok"
		}

		targets = append(targets, map[string]any{
			"symbol":          symbol,
			"regime":          regime,
			"reason":          reason,
			"score":           measurement.Confidence,
			"effective_score": measurement.Confidence,
			"trail_pct":       0,
			"source":          measurement.Source,
		})
	}

	return targets
}

func (crypto *Crypto) publishDecisionTrace(
	batch []engine.Measurement,
	candidates []tradeCandidate,
	line entryLine,
) {
	if crypto.publisher == nil {
		return
	}

	candidateBySymbol := make(map[string]tradeCandidate, len(candidates))
	peakConfidence := 0.0

	for _, candidate := range candidates {
		candidateBySymbol[candidate.symbol] = candidate

		if candidate.confidence > peakConfidence {
			peakConfidence = candidate.confidence
		}
	}

	readingsBySymbol := groupReadingsBySymbol(batch)
	evaluations := make([]map[string]any, 0, len(candidates))
	decisions := make([]map[string]any, 0, len(batch)+len(candidates))
	seen := make(map[string]struct{}, len(batch))

	for _, measurement := range batch {
		if measurement.Err != nil || len(measurement.Pairs) == 0 {
			continue
		}

		symbol := pairSymbol(measurement.Pairs[0])
		if symbol == "" {
			continue
		}

		seen[symbol] = struct{}{}

		regime := measurement.Regime
		if regime == "" {
			regime = regimeForType(measurement.Type)
		}

		reason := measurement.Reason
		if reason == "" {
			reason = "ok"
		}

		candidate, inPlay := candidateBySymbol[symbol]
		allow := false
		why := "signal_only"

		if inPlay {
			allow = crypto.candidateAllowsEntry(candidate, line, peakConfidence)
			why = entryWhy(candidate, line, crypto)

			if _, held := crypto.holds[candidate.symbol]; held {
				why = "stop_cooldown"
				allow = false
			}
		}

		score := measurement.Confidence
		if inPlay {
			score = candidate.confidence
		}

		decisions = append(decisions, map[string]any{
			"symbol":          symbol,
			"regime":          regime,
			"reason":          reason,
			"score":           score,
			"in_play":         inPlay,
			"allow":           allow,
			"why":             why,
			"confidence":      score,
			"effective_score": score,
			"source":          measurement.Source,
		})
	}

	for _, candidate := range candidates {
		if _, ok := seen[candidate.symbol]; ok {
			continue
		}

		allow := crypto.candidateAllowsEntry(candidate, line, peakConfidence)
		why := entryWhy(candidate, line, crypto)

		if _, held := crypto.holds[candidate.symbol]; held {
			why = "stop_cooldown"
			allow = false
		}

		decisions = append(decisions, map[string]any{
			"symbol":          candidate.symbol,
			"regime":          candidate.regime,
			"reason":          candidate.reason,
			"score":           candidate.confidence,
			"in_play":         true,
			"allow":           allow,
			"why":             why,
			"confidence":      candidate.confidence,
			"effective_score": candidate.confidence,
		})
	}

	for _, candidate := range candidates {
		evaluations = append(evaluations, map[string]any{
			"symbol":   candidate.symbol,
			"combined": candidate.confidence,
			"support":  candidate.support,
			"regime":   candidate.regime,
			"reason":   candidate.reason,
			"allow":    crypto.candidateAllowsEntry(candidate, line, peakConfidence),
			"why":      entryWhy(candidate, line, crypto),
			"signals":  readingsBySymbol[candidate.symbol],
		})
	}

	allowed := 0

	for _, row := range decisions {
		allow, ok := row["allow"].(bool)
		if ok && allow {
			allowed++
		}
	}

	crypto.publisher.Emit(map[string]any{
		"event":       "decision_trace",
		"ts":          time.Now().UTC().Format(time.RFC3339Nano),
		"line":        line.line,
		"median":      line.median,
		"mad":         line.mad,
		"scored":      len(batch),
		"in_play":     len(candidates),
		"allowed":     allowed,
		"decisions":   decisions,
		"evaluations": evaluations,
	})
}

func groupReadingsBySymbol(batch []engine.Measurement) map[string][]map[string]any {
	grouped := make(map[string][]map[string]any)

	for _, measurement := range batch {
		if measurement.Err != nil || measurement.Confidence <= 0 || len(measurement.Pairs) == 0 {
			continue
		}

		symbol := pairSymbol(measurement.Pairs[0])
		if symbol == "" {
			continue
		}

		regime := measurement.Regime
		if regime == "" {
			regime = regimeForType(measurement.Type)
		}

		reason := measurement.Reason
		if reason == "" {
			reason = "ok"
		}

		grouped[symbol] = append(grouped[symbol], map[string]any{
			"source":     measurement.Source,
			"regime":     regime,
			"reason":     reason,
			"confidence": measurement.Confidence,
		})
	}

	return grouped
}

func (crypto *Crypto) candidateAllowsEntry(
	candidate tradeCandidate,
	line entryLine,
	peakConfidence float64,
) bool {
	if !crypto.meetsEntryLine(candidate, line) {
		return false
	}

	if !crypto.canEnter(candidate) {
		return false
	}

	if peakConfidence <= 0 {
		peakConfidence = candidate.confidence
	}

	return crypto.entryNotional(candidate.confidence, peakConfidence) > 0
}

func entryWhy(candidate tradeCandidate, line entryLine, crypto *Crypto) string {
	if !crypto.meetsEntryLine(candidate, line) {
		return "below_line"
	}

	if !crypto.canEnter(candidate) {
		return "slot_limit"
	}

	return "ok"
}
