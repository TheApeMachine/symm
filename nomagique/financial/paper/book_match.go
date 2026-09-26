package paper

import (
	"bytes"
	"encoding/json"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
)

/* match attributes one trade only to the reconciled touch already observed. */
func (server *BookServer) match(data json.RawMessage) error {
	data = bytes.TrimSpace(data)

	if bytes.HasPrefix(data, []byte("[")) {
		var records []json.RawMessage

		if err := json.Unmarshal(data, &records); err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "paper.book: trade records", err))
		}
		if len(records) != 1 {
			return errnie.Error(errnie.Err(errnie.Validation, "paper.book: one evaluation requires one trade", nil))
		}
		data = records[0]
	}
	var trade struct {
		Symbol string          `json:"symbol"`
		Side   string          `json:"side"`
		Price  json.RawMessage `json:"price"`
		Qty    json.RawMessage `json:"qty"`
	}

	if err := json.Unmarshal(data, &trade); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "paper.book: trade record", err))
	}
	if trade.Symbol == "" || (trade.Side != "buy" && trade.Side != "sell") {
		return errnie.Error(errnie.Err(errnie.Validation, "paper.book: trade requires symbol and buy/sell side", nil))
	}
	price, err := core.ReadDecimal(trade.Price, "trade price")

	if err != nil {
		return err
	}
	quantity, err := core.ReadDecimal(trade.Qty, "trade quantity")

	if err != nil {
		return err
	}
	if price.Sign() <= 0 || quantity.Sign() <= 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "paper.book: trade price and quantity must be positive", nil))
	}
	server.symbol = trade.Symbol
	market := server.markets[trade.Symbol]

	if market == nil || market.book == nil {
		return nil
	}
	held := market.book

	if held == nil || held.BestBid() == nil || held.BestAsk() == nil {
		return nil
	}
	bid, ask := held.BestBid(), held.BestAsk()

	if price.Cmp(bid.Price) >= 0 && price.Cmp(ask.Price) <= 0 {
		server.values[10] = quantity.Float64()
	}
	if trade.Side == "sell" && price.Cmp(bid.Price) == 0 {
		server.values[11], server.values[13] = quantity.Float64(), 1
		market.match(0, quantity)
	}
	if trade.Side == "buy" && price.Cmp(ask.Price) == 0 {
		server.values[12], server.values[14] = quantity.Float64(), 1
		market.match(1, quantity)
	}
	server.values[15], server.values[16] = bid.Quantity.Float64(), ask.Quantity.Float64()
	for index := 10; index < 17; index++ {
		server.present[index] = true
	}
	return nil
}
