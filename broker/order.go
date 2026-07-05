package broker

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
)

/*
OrderFactory translates trader candidate actions into Kraken private requests.
It does not decide whether a strategy is good; it only validates and encodes.
*/
type OrderFactory struct {
	quote string
}

type orderCandidate struct {
	Symbol       string
	Side         string
	ActionType   logic.ActionType
	OrderType    string
	Quantity     float64
	LimitPrice   float64
	TriggerPrice float64
	TrailOffset  float64
	Notional     float64
	DecisionID   string
	ActionID     string
	SetupKey     string
}

/*
NewOrderFactory instantiates an order factory using the configured quote asset.
*/
func NewOrderFactory() *OrderFactory {
	quote := strings.ToUpper(strings.TrimSpace(viper.GetString("market.quote_currency")))
	if quote == "" {
		quote = "USD"
	}

	return &OrderFactory{quote: quote}
}

/*
Build converts one allowed action into one add_order artifact and pending row.
*/
func (factory *OrderFactory) Build(
	action *logic.Action,
	balances *BalanceBook,
	ticker *Ticker,
) (*websocket.OrderRequest, PendingOrder, error) {
	candidate, err := factory.candidate(action, balances, ticker)
	if err != nil {
		return nil, PendingOrder{}, err
	}

	orderID := uuid.NewString()
	order, err := websocket.NewOrderRequest(
		"add_order",
		factory.params(candidate, orderID),
		time.Now().UnixNano(),
	)
	if err != nil {
		return nil, PendingOrder{}, err
	}

	pending := PendingOrder{
		ClOrdID:    orderID,
		DecisionID: candidate.DecisionID,
		ActionID:   candidate.ActionID,
		Symbol:     candidate.Symbol,
		Side:       candidate.Side,
		OrderType:  candidate.OrderType,
		Qty:        candidate.Quantity,
		Notional:   candidate.Notional,
		CreatedAt:  time.Now().UTC(),
		Protective: candidate.ActionType.Protective() || candidate.ActionType.IsExit(),
	}

	return order, pending, nil
}

func (factory *OrderFactory) candidate(
	action *logic.Action,
	balances *BalanceBook,
	ticker *Ticker,
) (orderCandidate, error) {
	if factory == nil {
		return orderCandidate{}, errnie.Error(errnie.Err(errnie.Validation, "order factory is nil", nil))
	}

	if action == nil {
		return orderCandidate{}, errnie.Error(errnie.Err(errnie.Validation, "broker: nil action", nil))
	}

	if balances == nil {
		return orderCandidate{}, errnie.Error(errnie.Err(errnie.Validation, "broker: balances unavailable", nil))
	}

	if ticker == nil {
		return orderCandidate{}, errnie.Error(errnie.Err(errnie.Validation, "broker: ticker unavailable", nil))
	}

	symbol := actionSymbol(action)
	if symbol == "" {
		return orderCandidate{}, errnie.Error(errnie.Err(errnie.Validation, "broker: action missing symbol", nil))
	}

	actionType := action.Type
	if actionType == "" || actionType == logic.ActionNone {
		return orderCandidate{}, errnie.Error(errnie.Err(errnie.Validation, "broker: action missing type for "+symbol, nil))
	}

	krakenType, err := actionType.KrakenOrderType()
	if err != nil {
		return orderCandidate{}, err
	}

	side := strings.ToLower(strings.TrimSpace(string(action.Side)))
	if actionType == logic.ActionSettlePosition {
		side = "sell"
	}

	if side != "buy" && side != "sell" {
		return orderCandidate{}, errnie.Error(errnie.Err(errnie.Validation, "broker: action side must be buy or sell for "+symbol, nil))
	}

	orderType := string(krakenType)
	if actionType == logic.ActionSettlePosition {
		orderType = string(logic.OrderMarket)
	}

	return factory.completeCandidate(action, balances, ticker, orderSeed{
		symbol:     symbol,
		side:       side,
		actionType: actionType,
		orderType:  orderType,
	})
}

type orderSeed struct {
	symbol     string
	side       string
	actionType logic.ActionType
	orderType  string
}

func (factory *OrderFactory) completeCandidate(
	action *logic.Action,
	balances *BalanceBook,
	ticker *Ticker,
	seed orderSeed,
) (orderCandidate, error) {
	quote, _ := ticker.Quote(seed.symbol)
	quantity, notional, err := factory.quantity(action, balances, quote, seed)
	if err != nil {
		return orderCandidate{}, err
	}

	limitPrice, err := factory.limitPrice(action, quote, seed)
	if err != nil {
		return orderCandidate{}, err
	}

	triggerPrice, err := factory.triggerPrice(action, seed)
	if err != nil {
		return orderCandidate{}, err
	}

	trailOffset, err := factory.trailingOffset(action, seed)
	if err != nil {
		return orderCandidate{}, err
	}

	return orderCandidate{
		Symbol:       seed.symbol,
		Side:         seed.side,
		ActionType:   seed.actionType,
		OrderType:    seed.orderType,
		Quantity:     quantity,
		LimitPrice:   limitPrice,
		TriggerPrice: triggerPrice,
		TrailOffset:  trailOffset,
		Notional:     notional,
		DecisionID:   actionStringFirst(action, []any{"decision_id"}, []any{"decision", "id"}),
		ActionID:     actionStringFirst(action, []any{"action_id"}),
		SetupKey:     setupKey(action),
	}, nil
}

func (factory *OrderFactory) params(candidate orderCandidate, orderID string) map[string]any {
	params := map[string]any{
		"symbol":      candidate.Symbol,
		"side":        candidate.Side,
		"order_type":  candidate.OrderType,
		"order_qty":   candidate.Quantity,
		"cl_ord_id":   orderID,
		"action_type": string(candidate.ActionType),
	}

	if candidate.LimitPrice > 0 {
		params["limit_price"] = candidate.LimitPrice
	}

	if candidate.TriggerPrice > 0 {
		params["trigger_price"] = candidate.TriggerPrice
	}

	if candidate.TrailOffset > 0 {
		params["trailing_stop"] = candidate.TrailOffset
	}

	if candidate.DecisionID != "" {
		params["decision_id"] = candidate.DecisionID
	}

	if candidate.ActionID != "" {
		params["action_id"] = candidate.ActionID
	}

	if candidate.SetupKey != "" {
		params["setup_key"] = candidate.SetupKey
	}

	return params
}
