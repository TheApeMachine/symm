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
		"event":           "status",
		"ts":              time.Now().UTC().Format(time.RFC3339Nano),
		"equity_eur":      equity,
		"cash_eur":        crypto.wallet.Balance,
		"closed_pnl_eur":  crypto.closedPnL,
		"trade_count":     crypto.tradeCount,
		"win_rate":        winRate,
		"open_count":      len(crypto.holds),
		"positions":       positions,
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

func (crypto *Crypto) publishDecisionTrace(
	batch []engine.Measurement,
	candidates []tradeCandidate,
	peakConfidence float64,
) {
	if crypto.publisher == nil {
		return
	}

	decisions := make([]map[string]any, 0, len(candidates))
	allowed := 0

	for _, candidate := range candidates {
		allow := crypto.canEnter(candidate) && crypto.entryNotional(candidate.confidence, peakConfidence) > 0
		why := "ok"

		if _, held := crypto.holds[candidate.symbol]; held {
			why = "stop_cooldown"
			allow = false
		}

		if !crypto.canEnter(candidate) {
			why = "below_line"
			allow = false
		}

		if allow {
			allowed++
		}

		decisions = append(decisions, map[string]any{
			"symbol":           candidate.symbol,
			"regime":           candidate.regime,
			"reason":           candidate.reason,
			"score":            candidate.confidence,
			"in_play":          true,
			"allow":            allow,
			"why":              why,
			"confidence":       candidate.confidence,
			"effective_score":  candidate.confidence,
		})
	}

	crypto.publisher.Emit(map[string]any{
		"event":     "decision_trace",
		"ts":        time.Now().UTC().Format(time.RFC3339Nano),
		"line":      peakConfidence,
		"median":    peakConfidence,
		"mad":       0,
		"scored":    len(batch),
		"in_play":   len(candidates),
		"allowed":   allowed,
		"decisions": decisions,
	})
}

func (crypto *Crypto) publishPriceTicks() {
	if crypto.publisher == nil {
		return
	}

	quoteReader, typed := crypto.prices.(QuoteReader)
	if !typed {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)

	for symbol := range crypto.holds {
		last, bid, ask, changePct, ok := quoteReader.Quote(symbol)
		if !ok {
			continue
		}

		at := now
		if timestamp, tsOK := quoteReader.Timestamp(symbol); tsOK && timestamp != "" {
			at = timestamp
		}

		crypto.publisher.Emit(map[string]any{
			"event":            "price_tick",
			"ts":               now,
			"symbol":           symbol,
			"last":             last,
			"bid":              bid,
			"ask":              ask,
			"change_pct_24h":   changePct,
			"at":               at,
		})
	}
}
