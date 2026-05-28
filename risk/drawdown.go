package risk

import "github.com/theapemachine/symm/wallet"

/*
Drawdown tracks portfolio peak equity for dampener calculations.
*/
type Drawdown struct {
	peakEquity float64
}

/*
UpdatePeakEquity records the portfolio high-water mark for drawdown dampening.
*/
func (drawdown *Drawdown) UpdatePeakEquity(equity float64) {
	if drawdown == nil || equity <= 0 {
		return
	}

	if drawdown.peakEquity <= 0 || equity > drawdown.peakEquity {
		drawdown.peakEquity = equity
	}
}

/*
Pct reports peak-to-current equity drawdown as a unit fraction.
*/
func (drawdown *Drawdown) Pct(tradingWallet *wallet.Wallet, marks map[string]float64) float64 {
	if drawdown == nil || tradingWallet == nil || drawdown.peakEquity <= 0 {
		return 0
	}

	equity := tradingWallet.MarkEquity(marks)

	if equity <= 0 || equity >= drawdown.peakEquity {
		return 0
	}

	return (drawdown.peakEquity - equity) / drawdown.peakEquity
}
