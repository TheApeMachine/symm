package broker

import (
	"github.com/shopspring/decimal"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
Wallet reads the live desk bankroll for the dashboard.
Cash is spot Balance quote only — never TradeBalance equity.
*/
type Wallet struct {
	venue     market.Venue
	positions *Cache
}

func NewWallet(venue market.Venue, positions *Cache) *Wallet {
	return &Wallet{venue: venue, positions: positions}
}

/*
Reading is {cash, unrealized, equity}.

Cash is spot quote. Unrealized and equity are desk-local: TradeBalance `n`/`e`
are margin accounting and stay stale on live (one-shot REST prime only). Spot
NAV is cash plus open-lot close-now proceeds; unrealized is fee-net lot PnL.
*/
func (wallet *Wallet) Reading() map[string]float64 {
	quote := system.Cfg.Market.QuoteCurrency
	cash := 0.0

	if wallet.venue != nil {
		if spot := wallet.spotQuote(quote); spot != nil {
			cash, _ = spot.Float64()
		}
	}

	unrealized := wallet.openPnL()

	return map[string]float64{
		"cash":       cash,
		"unrealized": unrealized,
		"equity":     cash + wallet.openHeld(),
	}
}

func (wallet *Wallet) openPnL() float64 {
	sum := 0.0

	for _, position := range wallet.positions.Open() {
		if position == nil {
			continue
		}

		position.mu.Lock()
		pnl := position.PnL
		position.mu.Unlock()

		sum += decimalFloat(pnl)
	}

	return sum
}

/*
openHeld is fee-net quote locked in open lots (close-now proceeds), or
entry notional + PnL when marks are not ready yet.
*/
func (wallet *Wallet) openHeld() float64 {
	sum := 0.0

	for _, position := range wallet.positions.Open() {
		if position == nil {
			continue
		}

		if proceeds := position.CloseNowProceeds(); proceeds != nil {
			sum += decimalFloat(proceeds)
			continue
		}

		position.mu.Lock()
		entry := position.Entry
		qty := position.Qty
		pnl := position.PnL
		position.mu.Unlock()

		if entry == nil || qty == nil || entry.Sign() <= 0 || qty.Sign() <= 0 {
			continue
		}

		cost := entry.Mul(*qty)
		sum += decimalFloat(&cost) + decimalFloat(pnl)
	}

	return sum
}

func (wallet *Wallet) spotQuote(quote string) *decimal.Decimal {
	if wallet.venue == nil || quote == "" {
		return nil
	}

	bal, ok := wallet.venue.Balance().(*kraken.Balance)

	if !ok || bal == nil {
		return nil
	}

	return bal.QuoteCash(quote)
}

func decimalFloat(value *decimal.Decimal) float64 {
	if value == nil {
		return 0
	}

	out, _ := value.Float64()
	return out
}
