package broker

import (
	"fmt"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/logic"
)

/*
SubmitAction routes one playbook action to the Kraken private execution bus.
*/
func (desk *Desk) SubmitAction(action *logic.Action, holdings *logic.Balances) error {
	if desk == nil || action == nil || desk.pool == nil {
		return nil
	}

	wire, skip, buildErr := desk.addOrderWire(action, holdings)

	if buildErr != nil {
		return errnie.Error(buildErr)
	}

	if skip {
		return nil
	}

	payload, marshalErr := marshalAddOrderPayload(wire)

	if marshalErr != nil {
		return errnie.Error(marshalErr)
	}

	return errnie.Error(desk.sendPrivateOrder(payload))
}

func (desk *Desk) addOrderWire(
	action *logic.Action,
	holdings *logic.Balances,
) (map[string]any, bool, error) {
	orderType, mapErr := krakenOrderType(action.Type)

	if mapErr != nil {
		return nil, false, mapErr
	}

	quantity := resolveActionQuantity(action, holdings)

	if quantity <= 0 && action.Type != logic.ActionSettlePosition {
		return nil, true, nil
	}

	wire := map[string]any{
		"cl_ord_id":   uuid.NewString(),
		"symbol":      action.Symbol,
		"side":        string(action.Side),
		"order_qty":   quantity,
		"order_type":  orderType,
		"limit_price": action.Price,
		"action_type": string(action.Type),
	}

	if action.Offset > 0 && isTriggeredOrderType(orderType) {
		wire["triggers"] = map[string]any{
			"price":      action.Offset,
			"price_type": "pct",
		}
	}

	entryQueuedAt := time.Time{}

	if !action.Type.IsExit() {
		entryQueuedAt = time.Now().UTC()

		if desk.entryTransitExpired(entryQueuedAt) {
			return nil, true, nil
		}
	}

	return wire, false, nil
}

func (desk *Desk) entryTransitExpired(queuedAt time.Time) bool {
	transitTTL := viper.GetDuration("trading.entry.transit_ttl")

	if transitTTL <= 0 || queuedAt.IsZero() {
		return false
	}

	return time.Since(queuedAt) > transitTTL
}

func marshalAddOrderPayload(wire map[string]any) ([]byte, error) {
	message, buildErr := types.NewKrakenMessage("add_order", wire, time.Now().UnixNano())

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

func krakenOrderType(actionType logic.ActionType) (string, error) {
	switch actionType {
	case logic.ActionLimit:
		return "limit", nil
	case logic.ActionMarket:
		return "market", nil
	case logic.ActionIceberg:
		return "iceberg", nil
	case logic.ActionStopLoss:
		return "stop-loss", nil
	case logic.ActionStopLossLimit:
		return "stop-loss-limit", nil
	case logic.ActionTakeProfit:
		return "take-profit", nil
	case logic.ActionTakeProfitLimit:
		return "take-profit-limit", nil
	case logic.ActionTrailingStop:
		return "trailing-stop", nil
	case logic.ActionTrailingStopLimit:
		return "trailing-stop-limit", nil
	case logic.ActionSettlePosition:
		if viper.GetBool("trading.margin_enabled") {
			return "settle-position", nil
		}

		return "market", nil
	default:
		return "", errnie.Err(
			errnie.Validation,
			fmt.Sprintf("desk: unsupported action type %q", actionType),
			nil,
		)
	}
}

func isTriggeredOrderType(orderType string) bool {
	switch orderType {
	case "stop-loss", "stop-loss-limit",
		"take-profit", "take-profit-limit",
		"trailing-stop", "trailing-stop-limit":
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
