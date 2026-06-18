package broker

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/logic"
)

/*
SubmitAction routes one playbook action to paper or live private execution.
*/
func (desk *Desk) SubmitAction(action *logic.Action, holdings *logic.Balances) error {
	if desk == nil || action == nil || desk.pool == nil {
		return nil
	}

	params, skip, buildErr := desk.addParamsForAction(action, holdings)

	if buildErr != nil {
		return errnie.Error(buildErr)
	}

	if skip || desk.shouldSkipPreflight(action, params) {
		return nil
	}

	payload, marshalErr := marshalAddOrderPayload(params)

	if marshalErr != nil {
		return errnie.Error(marshalErr)
	}

	return errnie.Error(desk.sendPrivateOrder(payload))
}

func (desk *Desk) addParamsForAction(
	action *logic.Action,
	holdings *logic.Balances,
) (trading.AddParams, bool, error) {
	orderType, mapErr := krakenOrderType(action.Type)

	if mapErr != nil {
		return trading.AddParams{}, false, mapErr
	}

	quantity := resolveActionQuantity(action, holdings)

	if quantity <= 0 && action.Type != logic.ActionSettlePosition {
		return trading.AddParams{}, true, nil
	}

	params := trading.AddParams{
		ClOrdID:    uuid.NewString(),
		Symbol:     action.Symbol,
		Side:       trading.Side(action.Side),
		OrderQty:   quantity,
		LimitPrice: action.Price,
		OrderType:  orderType,
	}

	if action.Offset > 0 && isTriggeredOrderType(orderType) {
		params.Triggers = &trading.Triggers{
			Price:     action.Offset,
			PriceType: "pct",
		}
	}

	if !action.Type.IsExit() {
		params.EntryQueuedAt = time.Now().UTC()

		if desk.entryTransitExpired(params.EntryQueuedAt) {
			return trading.AddParams{}, true, nil
		}
	}

	return params, false, nil
}

func (desk *Desk) shouldSkipPreflight(action *logic.Action, params trading.AddParams) bool {
	if desk == nil || action == nil || !paperTradingEnabled() {
		return false
	}

	if desk.tree == nil || desk.quotes == nil {
		return false
	}

	quote, ok := desk.quotes.QuoteForSymbol(action.Symbol)

	if !ok {
		return true
	}

	orderType, mapErr := krakenOrderType(action.Type)

	if mapErr != nil {
		return true
	}

	preflightErr := PreflightGates(PreflightRequest{
		Quote:      quote,
		Side:       params.Side,
		Quantity:   params.OrderQty,
		OrderType:  orderType,
		ActionType: action.Type,
	})

	return preflightErr != nil
}

func (desk *Desk) entryTransitExpired(queuedAt time.Time) bool {
	transitTTL := viper.GetDuration("trading.entry.transit_ttl")

	if transitTTL <= 0 || queuedAt.IsZero() {
		return false
	}

	return time.Since(queuedAt) > transitTTL
}

func paperTradingEnabled() bool {
	if strings.TrimSpace(os.Getenv("SYMM_LIVE")) == "1" {
		return false
	}

	model := strings.ToLower(strings.TrimSpace(viper.GetString("trading.model")))

	return model == "" || model == "paper"
}

func marshalAddOrderPayload(params trading.AddParams) ([]byte, error) {
	message, buildErr := types.NewKrakenMessage(
		trading.MethodAddOrder,
		params,
		time.Now().UnixNano(),
	)

	if buildErr != nil {
		return nil, buildErr
	}

	payload, marshalErr := sonic.Marshal(message)

	if marshalErr != nil {
		return nil, errnie.Err(
			errnie.Validation,
			"desk: failed to marshal add_order",
			marshalErr,
		)
	}

	return payload, nil
}

func (desk *Desk) sendPrivateOrder(payload []byte) error {
	artifact := datura.Acquire("trader", datura.Artifact_Type_json).
		WithDestination("kraken:private").
		WithRole("orders").
		WithPayload(payload)

	return desk.pool.CreateBroadcastGroup("kraken:private").Send(artifact)
}

func krakenOrderType(actionType logic.ActionType) (trading.OrderType, error) {
	switch actionType {
	case logic.ActionLimit:
		return trading.Limit, nil
	case logic.ActionMarket:
		return trading.Market, nil
	case logic.ActionIceberg:
		return trading.Iceberg, nil
	case logic.ActionStopLoss:
		return trading.StopLoss, nil
	case logic.ActionStopLossLimit:
		return trading.StopLossLimit, nil
	case logic.ActionTakeProfit:
		return trading.TakeProfit, nil
	case logic.ActionTakeProfitLimit:
		return trading.TakeProfitLimit, nil
	case logic.ActionTrailingStop:
		return trading.TrailingStop, nil
	case logic.ActionTrailingStopLimit:
		return trading.TrailingStopLimit, nil
	case logic.ActionSettlePosition:
		if viper.GetBool("trading.margin_enabled") {
			return trading.SettlePosition, nil
		}

		return trading.Market, nil
	default:
		return "", errnie.Err(
			errnie.Validation,
			fmt.Sprintf("desk: unsupported action type %q", actionType),
			nil,
		)
	}
}

func isTriggeredOrderType(orderType trading.OrderType) bool {
	switch orderType {
	case trading.StopLoss, trading.StopLossLimit,
		trading.TakeProfit, trading.TakeProfitLimit,
		trading.TrailingStop, trading.TrailingStopLimit:
		return true
	default:
		return false
	}
}

func resolveActionQuantity(
	action *logic.Action,
	holdings *logic.Balances,
) float64 {
	if action == nil {
		return 0
	}

	if action.Quantity > 0 {
		return action.Quantity
	}

	if action.Fraction <= 0 || holdings == nil || action.Symbol == "" {
		return action.Quantity
	}

	baseAsset := symbolBaseAsset(action.Symbol)

	if baseAsset == "" {
		return 0
	}

	held := holdings.Inventory[baseAsset]

	if held <= 0 {
		held = holdings.Inventory[action.Symbol]
	}

	if held <= 0 {
		return 0
	}

	return held * action.Fraction
}

func symbolBaseAsset(symbol string) string {
	parts := strings.Split(symbol, "/")

	if len(parts) != 2 {
		return ""
	}

	return strings.ToUpper(parts[0])
}
