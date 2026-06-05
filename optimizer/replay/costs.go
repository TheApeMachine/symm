package replay

import (
	"strings"
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
	DefaultStopLossPctPercent         = 2.0
	DefaultTakeProfitPctPercent       = 3.0
	DefaultTrailingVolatilityMultiple = 3.0
	// DefaultStartingCapital is the account size when no paper wallet is configured.
	DefaultStartingCapital = 200.0
	// DefaultPositionFraction deploys the whole available balance per entry — the
	// realistic model for a small single-currency account.
	DefaultPositionFraction = 1.0
)

/*
ReplayCosts models per-side execution drag and the account the strategy trades
within. Fees and trigger offsets are fractions (0.004 = 0.40%).

The account fields make funding a first-class constraint: entries are sized from
available cash in WalletCurrency, so a strategy that wants to be in ten positions
at once on a single small wallet is scored on only the trades it could actually
fund — exactly as live.
*/
type ReplayCosts struct {
	MakerFeePct                float64
	TakerFeePct                float64
	SlippagePct                float64
	StopLossPct                float64 // long exits if price falls this fraction below entry
	TakeProfitPct              float64 // long exits if price rises this fraction above entry
	TrailingVolatilityMultiple float64
	StartingCapital            float64 // account balance the replay starts with
	PositionFraction           float64 // fraction of available cash deployed per entry
	WalletCurrency             string  // quote currency the wallet holds; only matching pairs are fundable
	ExecutionLatency           time.Duration
	ExecutionStressEnabled     bool
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

	walletCurrency := strings.ToUpper(config.GetString("market.quote_currency"))

	startingCapital := config.GetFloat64("trading.paper.wallet_" + strings.ToLower(walletCurrency))

	if startingCapital <= 0 {
		startingCapital = DefaultStartingCapital
	}

	positionFraction := config.GetFloat64("trading.position_fraction")

	if positionFraction <= 0 || positionFraction > 1 {
		positionFraction = DefaultPositionFraction
	}

	return ReplayCosts{
		MakerFeePct:                makerPct / 100.0,
		TakerFeePct:                takerPct / 100.0,
		SlippagePct:                slippageBps / 10000.0,
		StopLossPct:                exitOffsetPctFromViper("trading.exit.stop_loss_pct", DefaultStopLossPctPercent),
		TakeProfitPct:              exitOffsetPctFromViper("trading.exit.take_profit_pct", DefaultTakeProfitPctPercent),
		TrailingVolatilityMultiple: trailingVolatilityMultipleFromViper(),
		StartingCapital:            startingCapital,
		PositionFraction:           positionFraction,
		WalletCurrency:             walletCurrency,
		ExecutionLatency:           replayExecutionLatencyFromViper(),
		ExecutionStressEnabled:     replayExecutionStressEnabledFromViper(),
	}
}

func trailingVolatilityMultipleFromViper() float64 {
	multiple := viper.GetViper().GetFloat64("trading.exit.trailing_volatility_multiple")

	if multiple <= 0 {
		return DefaultTrailingVolatilityMultiple
	}

	return multiple
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
