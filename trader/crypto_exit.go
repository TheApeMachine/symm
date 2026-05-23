package trader

import (
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
)

type symbolRecord struct {
	wins int
}

/*
markExits closes held positions on stop ratchet, profit target, or trailing stop breach.
*/
func (crypto *Crypto) markExits() error {
	if crypto.prices == nil {
		return nil
	}

	for symbol, hold := range crypto.holds {
		exitFill, ok := crypto.exitFill(symbol)

		if !ok || exitFill <= 0 || hold.entryPrice <= 0 {
			continue
		}

		if exitFill > hold.peakPrice {
			oldStop := hold.stopPrice
			hold.peakPrice = exitFill
			hold.stopPrice = stopFromEntry(hold.peakPrice, hold.trailPct)
			crypto.holds[symbol] = hold

			if crypto.publisher != nil && hold.stopPrice > oldStop {
				crypto.publisher.Emit(map[string]any{
					"event":    "stop_ratchet",
					"ts":       time.Now().UTC().Format(time.RFC3339Nano),
					"symbol":   symbol,
					"old_stop": oldStop,
					"new_stop": hold.stopPrice,
					"peak":     hold.peakPrice,
					"last":     exitFill,
				})
			}
		}

		if exitFill <= hold.stopPrice {
			crypto.closePosition(symbol, hold, exitFill, "stop_ratchet")
			continue
		}

		if time.Since(hold.enteredAt) < crypto.minHoldForRegime(hold.regime) {
			continue
		}

		proceeds := hold.notional * (exitFill / hold.entryPrice)
		exitFee := config.System.TakerFee(proceeds, crypto.wallet.FeePct)
		credit := proceeds - exitFee
		cost := hold.notional + hold.entryFee
		profitPct := (credit - cost) / cost
		target := minProfitPct(hold.trailPct)

		if profitPct < target {
			continue
		}

		crypto.closePosition(symbol, hold, exitFill, "take_profit")
	}

	return nil
}

func (crypto *Crypto) closePosition(symbol string, hold position, exitFill float64, reason string) {
	proceeds := hold.notional * (exitFill / hold.entryPrice)
	exitFee := config.System.TakerFee(proceeds, crypto.wallet.FeePct)
	credit := proceeds - exitFee
	cost := hold.notional + hold.entryFee
	pnl := credit - cost

	crypto.wallet.Balance += credit
	delete(crypto.holds, symbol)

	crypto.tradeCount++

	if pnl > 0 {
		record := crypto.records[symbol]
		record.wins++
		crypto.records[symbol] = record
		crypto.winCount++
	}

	crypto.closedPnL += pnl

	errnie.Info(fmt.Sprintf(
		"paper_exit symbol=%s regime=%s proceeds=%.4f pnl=%.4f wins=%d reason=%s",
		symbol, hold.regime, credit, pnl, crypto.records[symbol].wins, reason,
	))

	if crypto.publisher != nil {
		crypto.publisher.Emit(map[string]any{
			"event":       "trade_exit",
			"ts":          time.Now().UTC().Format(time.RFC3339Nano),
			"symbol":      symbol,
			"regime":      hold.regime,
			"reason":      reason,
			"pnl_eur":     pnl,
			"hold_ms":     time.Since(hold.enteredAt).Milliseconds(),
			"entry_price": hold.entryPrice,
			"stop_price":  hold.stopPrice,
			"peak_price":  hold.peakPrice,
		})
	}
}

func repeatBoost(wins int) float64 {
	if wins <= 0 {
		return 1
	}

	return 1 + float64(wins)/float64(wins+1)
}

func (crypto *Crypto) boostConfidence(symbol string, confidence float64, regime string) float64 {
	if confidence <= 0 || regime != "pump" {
		return confidence
	}

	return confidence * repeatBoost(crypto.records[symbol].wins)
}
