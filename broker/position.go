package broker

import (
	"sort"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

type StopData struct {
	Symbol     string          `json:"symbol"`
	Armed      bool            `json:"-"`
	PeakPrice  decimal.Decimal `json:"-"`
	StopPrice  decimal.Decimal `json:"stop_price"`
	PeakReturn float64         `json:"peak_return"`
	StopReturn float64         `json:"stop_return"`
}

type Position struct {
	status    types.Status
	api       *websocket.API
	price     *Price
	balance   *Balance
	Stop      *StopData
	tickers   []*kraken.TickerData
	pnl       *decimal.Decimal
	returnPct *float64
	mark      *decimal.Decimal
}

func NewPosition(
	api *websocket.API,
	ui chan []byte,
	price *Price,
	balance *Balance,
	order *spot.Order,
) *Position {
	position := &Position{
		status:  types.INITIALIZING,
		api:     api,
		price:   price,
		balance: balance,
		tickers: make([]*kraken.TickerData, 0),
		Stop:    &StopData{Symbol: order.Description.Pair},
	}

	position.api.On("add_order", position.OrderAck)
	position.api.On("executions", position.ExecutionAck)
	position.api.On("ticker", position.TickerAck)

	return position
}

func (position *Position) Status() types.Status {
	return position.status
}

/*
Hydrate connects a position to an existing wallet holding and its
corresponding buy trade.

Price owns the valuation calculation. Position only stores the result.

The current ticker may not have arrived yet this early in boot, since
ticker subscriptions for the whole tradable universe are still in
flight. A missing quote does not mean the holding is not real, so the
position still opens on its confirmed quantity and entry price; Mark,
PnL and ReturnPct stay at whatever Price already computed (or zero) and
self-correct the moment TickerAck sees this symbol.
*/
func (position *Position) Hydrate(
	symbol string,
	history *kraken.TradesHistory,
) *Position {
	if errnie.Error(kraken.Validate(history)) != nil {
		return position
	}

	if position.balance == nil || position.price == nil {
		return position
	}

	holding, err := position.balance.Holding(symbol)

	if errnie.Error(err) != nil {
		return position
	}

	if holding.Order == nil {
		holding.Order = &spot.Order{
			Description: &spot.OrderDescription{Pair: symbol},
			Volume:      &holding.Qty,
		}
	}

	if err := position.reconcile(history, symbol, holding); err != nil {
		errnie.Error(err)

		return position
	}

	holding, err = position.balance.Holding(symbol)

	if errnie.Error(err) != nil {
		return position
	}

	if holding.Order == nil || holding.Order.Description == nil ||
		holding.Order.Price == nil || holding.Order.Volume == nil {
		position.status = types.OPEN

		return position
	}

	holding.EntryPrice = *holding.Order.Price
	position.balance.Update(symbol, holding)

	quote, err := position.price.PositionQuote(
		holding.Order.Description.Pair,
		*holding.Order.Price,
		*holding.Order.Volume,
	)

	if err != nil {
		errnie.Warn(
			"position quote pending for " + holding.Order.Description.Pair + ": " + err.Error(),
		)
	} else {
		position.mark = &quote.Mark
		position.pnl = &quote.PnL
		position.returnPct = &quote.ReturnPct
		holding.Mark = quote.Mark
		holding.EntryPrice = *holding.Order.Price
		holding.EntryFee = quote.EntryFee
		holding.ExitFee = quote.ExitFee
		holding.PnL = quote.PnL
		holding.ReturnPct = quote.ReturnPct
		position.balance.Update(symbol, holding)
	}

	position.status = types.OPEN
	return position
}

/*
reconcile matches chronological sells against earlier buys and retains only the
actual buy lots still represented by the wallet holding. This prevents closed
round trips from contaminating the cost basis of a newly managed position.
*/
func (position *Position) reconcile(
	history *kraken.TradesHistory,
	symbol string,
	holding types.Holding,
) error {
	trades := make([]struct {
		id    string
		trade spot.Trade
	}, 0)

	for id, trade := range history.Result.Trades {
		if !position.balance.TradeMatchesSymbol(trade.Pair, symbol) {
			continue
		}

		if (trade.Type != "buy" && trade.Type != "sell") || trade.Price == nil ||
			trade.Cost == nil || trade.Fee == nil || trade.Volume == nil ||
			trade.Time == nil || trade.Volume.Sign() <= 0 {
			return errnie.Err(errnie.Validation, "incomplete trade history for "+symbol, nil)
		}

		trades = append(trades, struct {
			id    string
			trade spot.Trade
		}{id: id, trade: trade})
	}

	sort.Slice(trades, func(left, right int) bool {
		order := trades[left].trade.Time.Cmp(trades[right].trade.Time)

		if order == 0 {
			return trades[left].id < trades[right].id
		}

		return order < 0
	})

	lots := make([]struct {
		id        string
		trade     spot.Trade
		remaining decimal.Decimal
	}, 0)

	for _, item := range trades {
		if item.trade.Type == "buy" {
			lots = append(lots, struct {
				id        string
				trade     spot.Trade
				remaining decimal.Decimal
			}{id: item.id, trade: item.trade, remaining: *item.trade.Volume.Copy()})

			continue
		}

		remaining := item.trade.Volume.Copy()

		for index := range lots {
			if remaining.Sign() <= 0 {
				break
			}

			consumed := remaining

			if lots[index].remaining.Cmp(remaining) < 0 {
				consumed = lots[index].remaining.Copy()
			}

			lots[index].remaining = *lots[index].remaining.Sub(consumed)
			remaining = remaining.Sub(consumed)
		}

		if remaining.Sign() > 0 {
			return errnie.Err(errnie.Validation, "sell exceeds known inventory for "+symbol, nil)
		}
	}

	quantityScale := int64(0)
	costScale := int64(0)

	for _, lot := range lots {
		if lot.remaining.GetScale() > quantityScale {
			quantityScale = lot.remaining.GetScale()
		}

		if lot.trade.Cost.GetScale() > costScale {
			costScale = lot.trade.Cost.GetScale()
		}
	}

	calculationScale := max(quantityScale, costScale)
	totalQuantity := decimal.NewFromInt64(0).SetScale(calculationScale)
	totalCost := decimal.NewFromInt64(0).SetScale(calculationScale)
	walletQuantity := holding.Qty.Copy().SetScale(calculationScale)

	for _, lot := range lots {
		if lot.remaining.Sign() <= 0 {
			continue
		}

		ratio := lot.remaining.Div(lot.trade.Volume)
		cost := lot.trade.Cost.Mul(ratio)
		fee := lot.trade.Fee.Mul(ratio)
		tradeAt := time.Unix(0, int64(lot.trade.Time.Float64()*float64(time.Second)))

		execution := &kraken.Execution{Data: []kraken.ExecutionData{{
			OrderID:     lot.trade.OrderID,
			ExecID:      lot.id,
			ExecType:    "trade",
			Symbol:      symbol,
			Side:        "buy",
			LastQty:     lot.remaining.Float64(),
			LastPrice:   *lot.trade.Price,
			Cost:        *cost,
			FeeUsdEquiv: *fee,
			Timestamp:   tradeAt,
			OrderStatus: "filled",
		}}}

		holding.Executions = append(holding.Executions, execution)
		totalQuantity = totalQuantity.Add(&lot.remaining)
		totalCost = totalCost.Add(cost)
	}

	if totalQuantity.Cmp(walletQuantity) != 0 || totalQuantity.Sign() <= 0 {
		return errnie.Err(errnie.Validation, "trade history does not reconcile wallet quantity for "+symbol, nil)
	}

	holding.Order.Volume = walletQuantity.SetScale(holding.Qty.GetScale())
	holding.Order.Price = totalCost.Div(totalQuantity)
	position.balance.Update(symbol, holding)

	return nil
}

func (position *Position) OrderAck(buf []byte) {
	orderAck := kraken.NewOrderResponse(buf)

	if errnie.Error(kraken.Validate(orderAck)) != nil {
		position.status = types.ERROR
		return
	}

	position.status = types.OPEN
}

func (position *Position) ExecutionAck(buf []byte) {
	execution := kraken.NewExecution(buf)

	if errnie.Error(kraken.Validate(execution)) != nil {
		position.status = types.ERROR
		return
	}

	matchedExecution := &kraken.Execution{
		Channel:  execution.Channel,
		Type:     execution.Type,
		Data:     make([]kraken.ExecutionData, 0),
		Sequence: execution.Sequence,
	}

	if position.Stop == nil {
		position.status = types.ERROR

		return
	}

	holding, err := position.balance.Holding(position.Stop.Symbol)

	if errnie.Error(err) != nil {
		position.status = types.ERROR

		return
	}

	for _, data := range execution.Data {
		if data.Symbol != position.Stop.Symbol {
			continue
		}

		matchedExecution.Data = append(matchedExecution.Data, data)
	}

	if len(matchedExecution.Data) > 0 {
		if err := position.Execution(matchedExecution); err != nil {
			position.status = types.ERROR

			return
		}

		holding.Executions = append(
			holding.Executions,
			matchedExecution,
		)
		entryQuantity := decimal.NewFromInt64(0).SetScale(decimal.DefaultScale)
		exitQuantity := decimal.NewFromInt64(0).SetScale(decimal.DefaultScale)
		entryCost := decimal.NewFromInt64(0).SetScale(decimal.DefaultScale)
		entryFee := decimal.NewFromInt64(0).SetScale(decimal.DefaultScale)
		mark := decimal.NewFromInt64(0).SetScale(decimal.DefaultScale)
		seen := make(map[string]struct{})

		for _, observed := range holding.Executions {
			for _, fill := range observed.Data {
				if fill.Symbol != position.Stop.Symbol || fill.ExecType != "trade" {
					continue
				}

				if fill.ExecID == "" || fill.LastQty <= 0 || fill.Cost.Sign() <= 0 {
					position.status = types.ERROR
					errnie.Error(errnie.Err(
						errnie.Validation,
						"incomplete execution data for "+position.Stop.Symbol,
						nil,
					))

					return
				}

				if _, exists := seen[fill.ExecID]; exists {
					continue
				}

				seen[fill.ExecID] = struct{}{}
				quantity := decimal.NewFromFloat64(fill.LastQty).SetScale(decimal.DefaultScale)
				mark = fill.LastPrice.Copy().SetScale(decimal.DefaultScale)

				switch fill.Side {
				case "buy":
					entryQuantity = entryQuantity.Add(quantity)
					entryCost = entryCost.Add(&fill.Cost)
					entryFee = entryFee.Add(&fill.FeeUsdEquiv)

					if holding.EntryAt.IsZero() || fill.Timestamp.Before(holding.EntryAt) {
						holding.EntryAt = fill.Timestamp
					}
				case "sell":
					exitQuantity = exitQuantity.Add(quantity)
					holding.ExitAt = fill.Timestamp
				default:
					position.status = types.ERROR
					errnie.Error(errnie.Err(
						errnie.Validation,
						"unsupported execution side for "+position.Stop.Symbol,
						nil,
					))

					return
				}
			}
		}

		if entryQuantity.Sign() <= 0 {
			position.status = types.ERROR
			errnie.Error(errnie.Err(
				errnie.Validation,
				"position has no entry execution for "+position.Stop.Symbol,
				nil,
			))

			return
		}

		if entryQuantity.Cmp(exitQuantity) < 0 {
			position.status = types.ERROR
			errnie.Error(errnie.Err(
				errnie.Validation,
				"executions exceed position quantity for "+position.Stop.Symbol,
				nil,
			))

			return
		}

		holding.Qty = *entryQuantity.Sub(exitQuantity)
		holding.EntryPrice = *entryCost.Div(entryQuantity)
		holding.EntryFee = *entryFee
		holding.Mark = *mark

		if holding.Order != nil {
			holding.Order.Price = &holding.EntryPrice
			holding.Order.Volume = &holding.Qty
		}

		if holding.Qty.Sign() <= 0 {
			position.status = types.CLOSED

			if holding.Order != nil && holding.Order.Description != nil {
				outcome, err := position.price.Reconcile(
					holding.Order.Description.Pair, holding.Executions,
				)

				if err != nil {
					errnie.Error(err)
				} else {
					position.mark = &outcome.Mark
					position.pnl = &outcome.PnL
					position.returnPct = &outcome.ReturnPct
					holding.Mark = outcome.Mark
					holding.EntryFee = outcome.EntryFee
					holding.ExitFee = outcome.ExitFee
					holding.PnL = outcome.PnL
					holding.ReturnPct = outcome.ReturnPct
				}
			}
		} else if holding.Order != nil && holding.Order.Description != nil {
			remainingFraction := holding.Qty.Div(entryQuantity)
			holding.EntryFee = *entryFee.Mul(remainingFraction)
			quote, err := position.price.PositionQuoteAt(
				holding.Order.Description.Pair,
				holding.EntryPrice,
				holding.Mark,
				holding.Qty,
			)

			if err == nil {
				pnl := quote.PnL.Add(&quote.EntryFee).Sub(&holding.EntryFee)
				returnPct := pnl.Div(&quote.EntryNotional).Mul(decimal.NewFromInt64(100))
				position.mark = &quote.Mark
				position.pnl = pnl
				position.returnPct = new(float64)
				*position.returnPct = returnPct.Float64()
				holding.Mark = quote.Mark
				holding.ExitFee = quote.ExitFee
				holding.PnL = *pnl
				holding.ReturnPct = returnPct.Float64()
				position.status = types.OPEN
			} else {
				errnie.Error(err)
			}
		}

		position.balance.Update(position.Stop.Symbol, holding)
	}
}

/*
Execution validates an execution belonging to this position.
Fee and PnL calculations do not belong here. Price owns those
calculations centrally.
*/
func (position *Position) Execution(
	execution *kraken.Execution,
) error {
	if errnie.Error(kraken.Validate(execution)) != nil {
		position.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"invalid execution",
			nil,
		))
	}

	return nil
}

func (position *Position) Enter() error {
	if position.Stop == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position symbol not set",
			nil,
		))
	}

	holding, err := position.balance.Holding(position.Stop.Symbol)

	if err != nil {
		return errnie.Error(err)
	}

	if holding.Order == nil || holding.Order.Volume == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"entry order not set for "+position.Stop.Symbol,
			nil,
		))
	}

	requestedQty := holding.Order.Volume

	amount, err := position.price.Taker(
		position.Stop.Symbol,
		*requestedQty,
	)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get taker: "+err.Error(),
			err,
		))
	}

	if !position.balance.Available(*amount) {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"insufficient balance",
			nil,
		))
	}

	order := kraken.NewMarketOrder(
		"buy",
		requestedQty.Float64(),
		position.Stop.Symbol,
	)

	position.status = types.PENDING

	if err := position.api.AddOrder(order); err != nil {
		position.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	return nil
}

func (position *Position) Exit(action string, quantity decimal.Decimal) error {
	if position.Stop == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position symbol not set",
			nil,
		))
	}

	holding, err := position.balance.Holding(position.Stop.Symbol)

	if err != nil {
		return errnie.Error(err)
	}

	if quantity.Sign() <= 0 || quantity.Cmp(&holding.Qty) > 0 {
		return errnie.Error(errnie.Err(
			errnie.Forbidden,
			"position does not contain requested sell quantity",
			nil,
		))
	}

	order := kraken.NewMarketOrder(
		"sell",
		quantity.Float64(),
		position.Stop.Symbol,
	)

	position.status = types.PENDING

	if err := position.api.AddOrder(order); err != nil {
		position.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	return nil
}

func (position *Position) Executions() []*kraken.Execution {
	if position.Stop == nil {
		return nil
	}

	holding, err := position.balance.Holding(position.Stop.Symbol)

	if err != nil {
		return nil
	}

	return holding.Executions
}

/*
TickerAck updates this position from its own ticker only.

Position does not perform fee, notional, PnL, or return calculations.
It delegates the entire valuation to Price.
*/
func (position *Position) TickerAck(buf []byte) {
	if position.status != types.OPEN && position.status != types.PARTIAL_FILLED &&
		position.status != types.FILLED {
		return
	}

	if position.Stop == nil {
		return
	}

	holding, err := position.balance.Holding(position.Stop.Symbol)

	if err != nil || holding.Order == nil || holding.Order.Description == nil ||
		holding.Order.Price == nil || holding.Order.Volume == nil {
		position.status = types.ERROR

		return
	}

	ticker := kraken.NewTicker(buf)

	if errnie.Error(kraken.Validate(ticker)) != nil {
		return
	}

	for _, tickerData := range ticker.Data {
		if tickerData.Symbol != position.Stop.Symbol ||
			tickerData.Last == nil ||
			holding.Order.Price.Sign() <= 0 ||
			holding.Order.Volume.Sign() <= 0 {
			continue
		}

		quote, err := position.price.PositionQuoteAt(
			position.Stop.Symbol,
			*holding.Order.Price,
			*tickerData.Last,
			*holding.Order.Volume,
		)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to get position quote",
				err,
			))

			return
		}

		position.mark = &quote.Mark
		position.pnl = &quote.PnL
		position.returnPct = &quote.ReturnPct
		holding.Mark = quote.Mark
		holding.EntryPrice = *holding.Order.Price
		holding.EntryFee = quote.EntryFee
		holding.ExitFee = quote.ExitFee
		holding.PnL = quote.PnL
		holding.ReturnPct = quote.ReturnPct
		position.balance.Update(position.Stop.Symbol, holding)

		return
	}
}
