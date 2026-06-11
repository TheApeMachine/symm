package market

import (
	"errors"
	"math"

	"github.com/theapemachine/errnie"
)

/*
Validate requires a complete symbol row with every field populated.
Pressure may be zero when trade flow is balanced.
*/
func (symbol *Symbol) Validate() error {
	if err := errnie.Error(errnie.Require(map[string]any{
		"name":    symbol.Name,
		"updated": symbol.Updated,
		"price":   symbol.Price,
		"value":   symbol.Value,
		"volume":  symbol.Volume,
	})); err != nil {
		return err
	}

	if math.IsNaN(symbol.Pressure) || math.IsInf(symbol.Pressure, 0) {
		return errnie.Error(errors.New("kraken: pressure is invalid"))
	}

	return nil
}

/*
Validate rejects ticker rows that cannot supply a resolvable price.

24-hour summary fields such as high, low, volume, and vwap may legitimately
be zero for illiquid pairs or book-triggered updates before a session print.
*/
func (ticker *TickerUpdate) Validate() error {
	if ticker.Symbol == "" {
		return errnie.Error(errors.New("symbol is required"))
	}

	price, err := ticker.ResolvePrice()

	if err != nil {
		return errnie.Error(err)
	}

	if ticker.Bid > 0 && ticker.Ask > 0 && ticker.Ask <= ticker.Bid {
		return errnie.Error(errors.New("ask must exceed bid"))
	}

	return errnie.Error(errnie.Require(map[string]any{
		"price": price,
	}))
}

/*
Validate rejects non-finite fields on a trade row.
*/
func (trade *TradeUpdate) Validate() error {
	return errnie.Error(errnie.Require(map[string]any{
		"symbol":    trade.Symbol,
		"side":      trade.Side,
		"price":     trade.Price,
		"qty":       trade.Qty,
		"ord_type":  trade.OrdType,
		"trade_id":  trade.TradeID,
		"timestamp": trade.Timestamp,
	}))
}
