package broker

import (
	"github.com/shopspring/decimal"
	"github.com/theapemachine/nomagique/market"
)

/*
Wallet reads the venue TradeBalance into the dashboard shape.
*/
type Wallet struct {
	venue market.Venue
}

func NewWallet(venue market.Venue) *Wallet {
	return &Wallet{venue: venue}
}

/*
Reading is {cash, unrealized, equity} from venue.TradeBalance.
*/
func (wallet *Wallet) Reading() map[string]float64 {
	book := market.NewTradeBalance(nil)

	if wallet.venue != nil {
		if frame := wallet.venue.TradeBalance(); frame != nil {
			frame.ToMarket(book)
		}
	}

	return map[string]float64{
		"cash":       decimalFloat(book.AvailableCash),
		"unrealized": decimalFloat(book.UnrealizedPnL),
		"equity":     decimalFloat(book.Equity),
	}
}

func decimalFloat(value *decimal.Decimal) float64 {
	if value == nil {
		return 0
	}

	out, _ := value.Float64()
	return out
}
