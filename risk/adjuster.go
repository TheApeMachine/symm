package risk

import (
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/wallet"
)

/*
Adjuster is the position-scale dampener computed for one candidate entry.
*/
type Adjuster struct {
	Dampener            float64
	Reason              string
	DrawdownPct         float64
	SystemicCorrelation float64
	HasSystemicMeasure  bool
}

/*
For computes the dampener for one candidate entry against open exposure.
*/
func (adjuster *Adjuster) For(
	portfolio *Portfolio,
	tradingWallet *wallet.Wallet,
	measurement engine.Measurement,
	openSymbols []string,
) Adjuster {
	result := Adjuster{Dampener: 1}

	if portfolio == nil || tradingWallet == nil || len(measurement.Pairs) == 0 {
		return result
	}

	symbol := measurement.Pairs[0].Wsname
	result.DrawdownPct = portfolio.DrawdownPct(tradingWallet)
	result.applyDrawdownDampener()

	systemicCorrelation, ok := portfolio.SystemicCorrelation(symbol, openSymbols)

	if !ok {
		return result
	}

	result.SystemicCorrelation = systemicCorrelation
	result.HasSystemicMeasure = true
	result.applySystemicDampener()

	return result
}

func (adjuster *Adjuster) applyDrawdownDampener() {
	limit := config.System.MaxPortfolioDrawdownPct

	if limit <= 0 || adjuster.DrawdownPct <= 0 {
		return
	}

	remaining := 1 - adjuster.DrawdownPct/limit
	adjuster.applyDampener(clampUnit(remaining), "drawdown_dampened")
}

func (adjuster *Adjuster) applySystemicDampener() {
	limit := config.System.MaxSymbolCorrelation

	if limit <= 0 || adjuster.SystemicCorrelation <= 0 {
		return
	}

	remaining := 1 - adjuster.SystemicCorrelation/limit
	adjuster.applyDampener(clampUnit(remaining), "covariance_dampened")
}

func (adjuster *Adjuster) applyDampener(candidate float64, reason string) {
	if candidate >= adjuster.Dampener {
		return
	}

	adjuster.Dampener = candidate
	adjuster.Reason = reason
}
