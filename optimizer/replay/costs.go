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
)

/*
ReplayCosts models per-side execution drag for offline replay scoring.
Fees are stored as fractions (0.004 = 0.40%).
*/
type ReplayCosts struct {
	MakerFeePct            float64
	TakerFeePct            float64
	SlippagePct            float64
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
		ExecutionLatency:       replayExecutionLatencyFromViper(),
		ExecutionStressEnabled: replayExecutionStressEnabledFromViper(),
	}
}

func (costs ReplayCosts) feePct(actionType perspectives.ActionType) float64 {
	if perspectives.IsMakerAction(actionType) {
		return costs.MakerFeePct
	}

	return costs.TakerFeePct
}
