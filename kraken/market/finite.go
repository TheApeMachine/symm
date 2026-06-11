package market

import (
	"github.com/theapemachine/errnie"
)

/*
Validate requires a complete symbol row with every field populated.
*/
func (symbol *Symbol) Validate() error {
	return errnie.Error(errnie.Require(map[string]any{
		"name":     symbol.Name,
		"updated":  symbol.Updated,
		"price":    symbol.Price,
		"value":    symbol.Value,
		"volume":   symbol.Volume,
		"pressure": symbol.Pressure,
	}))
}

/*
Validate rejects non-finite fields on a ticker row.
*/
func (ticker *TickerUpdate) Validate() error {
	return errnie.Error(errnie.Require(map[string]any{
		"ask":     ticker.Ask,
		"ask_qty": ticker.AskQty,
		"bid":     ticker.Bid,
		"bid_qty": ticker.BidQty,
		"high":    ticker.High,
		"last":    ticker.Last,
		"low":     ticker.Low,
		"volume":  ticker.Volume,
		"vwap":    ticker.VWAP,
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
