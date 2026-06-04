package replay

import (
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives"
)

const (
	// DefaultTakerFeePctPercent matches kraken/paper/catalog lowest-tier taker fee.
	DefaultTakerFeePctPercent = 0.40
	// DefaultMakerFeePctPercent matches kraken/paper/catalog lowest-tier maker fee.
	DefaultMakerFeePctPercent = 0.25
	// DefaultSlippageBps is half-spread crossing per side when tape SpreadBPS is zero.
	DefaultSlippageBps = 5.0
	// Default protective-exit trigger offsets (percent of entry / peak). These make
	// stop-loss, take-profit and trailing-stop exits economically distinct from a
	// discretionary settle, so the optimizer can actually choose between them.
	DefaultStopLossPctPercent   = 2.0
	DefaultTakeProfitPctPercent = 3.0
	DefaultTrailingPctPercent   = 1.5
)

/*
ReplayCosts models per-side execution drag for offline replay scoring.
Fees and trigger offsets are stored as fractions (0.004 = 0.40%).
*/
type ReplayCosts struct {
	MakerFeePct            float64
	TakerFeePct            float64
	SlippagePct            float64
	StopLossPct            float64 // long exits if price falls this fraction below entry
	TakeProfitPct          float64 // long exits if price rises this fraction above entry
	TrailingPct            float64 // long exits if price falls this fraction below the peak
	ExecutionLatency       time.Duration
	ExecutionStressEnabled bool
}

/*
DefaultReplayCosts returns replay drag from trading.paper.* config when set,
otherwise Kraken spot lowest-tier maker/taker defaults.
*/
func DefaultReplayCosts() ReplayCosts {
	return ReplayCostsFromViper()
}

/*
ReplayCostsFromViper loads optimizer replay fees from config.

Supported keys (percent per side unless noted):
  - trading.paper.taker_fee_pct
  - trading.paper.fee_pct (alias for taker_fee_pct)
  - trading.paper.maker_fee_pct
  - trading.paper.slippage_bps (half-spread per side when SpreadBPS is absent)
*/
func ReplayCostsFromViper() ReplayCosts {
	config := viper.GetViper()

	takerPct := config.GetFloat64("trading.paper.taker_fee_pct")

	if takerPct <= 0 {
		takerPct = config.GetFloat64("trading.paper.fee_pct")
	}

	if takerPct <= 0 {
		takerPct = DefaultTakerFeePctPercent
	}

	makerPct := config.GetFloat64("trading.paper.maker_fee_pct")

	if makerPct <= 0 {
		makerPct = DefaultMakerFeePctPercent
	}

	slippageBps := config.GetFloat64("trading.paper.slippage_bps")

	if slippageBps <= 0 {
		slippageBps = DefaultSlippageBps
	}

	return ReplayCosts{
		MakerFeePct:            makerPct / 100.0,
		TakerFeePct:            takerPct / 100.0,
		SlippagePct:            slippageBps / 10000.0,
		StopLossPct:            exitOffsetPctFromViper("trading.exit.stop_loss_pct", DefaultStopLossPctPercent),
		TakeProfitPct:          exitOffsetPctFromViper("trading.exit.take_profit_pct", DefaultTakeProfitPctPercent),
		TrailingPct:            exitOffsetPctFromViper("trading.exit.trailing_pct", DefaultTrailingPctPercent),
		ExecutionLatency:       replayExecutionLatencyFromViper(),
		ExecutionStressEnabled: replayExecutionStressEnabledFromViper(),
	}
}

func exitOffsetPctFromViper(key string, fallbackPercent float64) float64 {
	percent := viper.GetViper().GetFloat64(key)

	if percent <= 0 {
		percent = fallbackPercent
	}

	return percent / 100.0
}

func (costs ReplayCosts) feePct(actionType perspectives.ActionType) float64 {
	if perspectives.IsMakerAction(actionType) {
		return costs.MakerFeePct
	}

	return costs.TakerFeePct
}
