package broker

import (
	"context"
	"encoding/hex"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/live"
	"github.com/theapemachine/symm/logic"
)

/*
Desk is the link between the trader and the Kraken exchange. It opens and closes
positions on the trader's command and protects them with trailing stops. It makes
no entry decisions of its own; the only call it makes alone is bailing out of a
position whose stop has been breached. Stop logic lives on Stoploss; Desk only
owns the live stop map and forwards resulting orders to Kraken.
*/
type Desk struct {
	ctx                       context.Context
	cancel                    context.CancelFunc
	pool                      *qpool.Q[any]
	tree                      *dmt.Tree
	broadcasts                *sync.Map
	orders                    *sync.Map
	stoplosses                *sync.Map
	marks                     *sync.Map
	quotes                    *sync.Map
	pending                   *sync.Map
	pendingByClOrdID          *sync.Map
	pendingByExchangeOrderID  *sync.Map
	pendingAckBySymbolSide    *sync.Map
	workingOrdersBySymbol     *sync.Map
	workingProtectiveBySymbol *sync.Map
	unprotected               *sync.Map
	subscribers               []*qpool.BroadcastConsumer
	balanceMu                 sync.RWMutex
	balances                  map[string]float64
	quote                     string
	closed                    atomic.Bool
	entryBlocked              atomic.Bool
	submittedCount            atomic.Int64
	preflightRejectedCount    atomic.Int64
	filledCount               atomic.Int64
}

func NewDesk(
	ctx context.Context, pool *qpool.Q[any], tree *dmt.Tree,
) *Desk {
	ctx, cancel := context.WithCancel(ctx)

	pendingByClOrdID := &sync.Map{}
	desk := &Desk{
		ctx:                       ctx,
		cancel:                    cancel,
		pool:                      pool,
		tree:                      tree,
		broadcasts:                &sync.Map{},
		orders:                    &sync.Map{},
		stoplosses:                &sync.Map{},
		marks:                     &sync.Map{},
		quotes:                    &sync.Map{},
		pending:                   pendingByClOrdID,
		pendingByClOrdID:          pendingByClOrdID,
		pendingByExchangeOrderID:  &sync.Map{},
		pendingAckBySymbolSide:    &sync.Map{},
		workingOrdersBySymbol:     &sync.Map{},
		workingProtectiveBySymbol: &sync.Map{},
		unprotected:               &sync.Map{},
		balances:                  make(map[string]float64),
		quote: strings.ToUpper(
			viper.GetString("market.quote_currency"),
		),
	}

	for _, channel := range []string{"kraken:private"} {
		desk.broadcasts.Store(channel, pool.CreateBroadcastGroup(channel))
	}

	for _, channel := range []string{"ticker", "executions", "balances"} {
		desk.subscribers = append(
			desk.subscribers, pool.Subscribe(channel, desk.onMessage),
		)
	}

	return desk
}

/*
Update converts each chosen action into a Kraken order request and sends it
to the kraken:private channel.
*/
func (desk *Desk) Update(
	chosen []*datura.Artifact,
) error {
	desk.checkPendingTimeouts()

	for _, action := range chosen {
		if !actionAllowedForDispatch(action) {
			desk.publishDiagnostic(action, "warning", diagnosticReason(action))
			continue
		}

		symbol, err := action.Scope()

		if err != nil || symbol == "" {
			continue
		}

		actionType := datura.Peek[string](action, "type")
		side := datura.Peek[string](action, "side")
		if strings.EqualFold(side, "buy") && desk.entryBlocked.Load() {
			desk.publishDiagnostic(action, "critical", "stop_exit_retry_exhausted")
			continue
		}
		if strings.EqualFold(side, "buy") && desk.symbolOpen(symbol) {
			desk.publishDiagnostic(action, "warning", "held")
			continue
		}
		if strings.EqualFold(side, "buy") && desk.nativeProtectionMissing(symbol) {
			desk.publishDiagnostic(action, "critical", "native_stop_missing")
			continue
		}

		qty := desk.resolveOrderQuantity(action, actionType, side, symbol)
		krakenType, krakenErr := logic.ActionType(actionType).KrakenOrderType()
		if krakenErr != nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"unknown order type from action",
				krakenErr,
			))

			continue
		}
		orderType := string(krakenType)
		if krakenType == logic.OrderSettlePosition {
			orderType = string(logic.OrderMarket)
		}

		if qty <= 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"broker: refusing zero-quantity order for "+symbol,
				nil,
			))
			continue
		}

		limitPrice := 0.0
		if orderType == "limit" || strings.HasSuffix(orderType, "-limit") {
			limitPrice = desk.limitPrice(action, side, symbol)
			if limitPrice <= 0 {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"broker: refusing limit order without price for "+symbol,
					nil,
				))
				continue
			}
		}
		triggerPrice := 0.0
		if requiresTriggerPrice(orderType) {
			triggerPrice = desk.triggerPrice(action, side, symbol)
			if triggerPrice <= 0 {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"broker: refusing protective order without trigger for "+symbol,
					nil,
				))
				continue
			}
		}
		trailingOffset := trailingOffsetForAction(action, orderType)
		if requiresTrailingOffset(orderType) && trailingOffset <= 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"broker: refusing trailing order without offset for "+symbol,
				nil,
			))
			continue
		}

		order := desk.newOrder(symbol, side, orderType, qty, limitPrice, triggerPrice, trailingOffset)
		if order == nil {
			continue
		}
		if setupKey := actionSetupKey(action); setupKey != "" {
			order.PokePayload(setupKey, "params", "setup_key")
		}
		if decisionID := datura.Peek[string](action, "decision_id"); decisionID != "" {
			order.PokePayload(decisionID, "params", "decision_id")
		}
		if actionID := datura.Peek[string](action, "action_id"); actionID != "" {
			order.PokePayload(actionID, "params", "action_id")
		}

		orderID, err := artifactOrderID(order)
		if err != nil {
			errnie.Error(err)
			order.Release()
			continue
		}
		action.Poke(orderID, "cl_ord_id")

		pending := &PendingOrder{
			ClOrdID:    orderID,
			DecisionID: datura.Peek[string](action, "decision_id"),
			ActionID:   datura.Peek[string](action, "action_id"),
			Symbol:     symbol,
			Side:       side,
			OrderType:  orderType,
			Qty:        qty,
			Notional:   datura.Peek[float64](action, "notional"),
			CreatedAt:  time.Now().UTC(),
			Protective: logic.ActionType(actionType).Protective(),
			Attempt:    1,
		}

		if !desk.storePending(pending) {
			order.Release()
			continue
		}

		desk.orders.Store(orderID, order)
		desk.sendPrivate(order)
	}

	return nil
}

func actionAllowedForDispatch(action *datura.Artifact) bool {
	if action == nil {
		return false
	}

	if datura.Peek[string](action, "verdict") == "blocked" ||
		datura.Peek[string](action, "decision", "verdict") == "blocked" {
		return false
	}

	if strings.EqualFold(datura.Peek[string](action, "side"), "buy") &&
		!datura.Peek[bool](action, "risk", "stamped") {
		return false
	}

	return datura.Peek[bool](action, "allowed")
}

func actionSetupKey(action *datura.Artifact) string {
	if action == nil {
		return ""
	}

	for _, path := range [][]any{
		{"decision", "setup_key"},
		{"decision", "edge_key"},
		{"edge", "key"},
		{"setup_key"},
		{"params", "setup_key"},
	} {
		if key := normalizeOrderSetupKey(datura.Peek[string](action, path...)); key != "" {
			return key
		}
	}

	source := datura.Peek[string](action, "reason_source")
	if source == "" {
		source = datura.Peek[string](action, "journey", "story", "source")
	}
	category := datura.Peek[string](action, "reason_category")
	if category == "" {
		category = datura.Peek[string](action, "journey", "story", "category")
	}
	side := datura.Peek[string](action, "side")
	actionType := datura.Peek[string](action, "type")
	if source == "" || category == "" || side == "" || actionType == "" {
		return ""
	}

	return normalizeOrderSetupKey(strings.Join([]string{source, category, side, actionType}, "|"))
}

func normalizeOrderSetupKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.Join(strings.Fields(key), "_")
	return key
}

/*
onMessage is called by the qpool.BroadcastGroup for every consumer
that has subscribed with a callback function.
*/
func (desk *Desk) onMessage(
	artifact *datura.Artifact,
) error {
	if desk == nil || artifact == nil || !artifact.IsValid() {
		return nil
	}
	desk.checkPendingTimeouts()

	role := datura.Peek[string](artifact, "role")
	symbol := datura.Peek[string](artifact, "scope")

	switch role {
	case "ticker":
		for rowIndex := 0; ; rowIndex++ {
			symbol := datura.Peek[string](artifact, "data", rowIndex, "symbol")
			if symbol == "" {
				break
			}

			last := datura.Peek[float64](artifact, "data", rowIndex, "last")
			if last <= 0 {
				continue
			}

			bid := datura.Peek[float64](artifact, "data", rowIndex, "bid")
			ask := datura.Peek[float64](artifact, "data", rowIndex, "ask")

			desk.marks.Store(symbol, last)
			desk.quotes.Store(symbol, marketQuote{
				bid:  bid,
				ask:  ask,
				last: last,
			})

			stoploss, ok := desk.stoplosses.Load(symbol)

			if ok {
				stoploss = stoploss.(*Stoploss).Ratchet(last)
				desk.publishStoploss(stoploss.(*Stoploss))

				if !live.Enabled() &&
					(stoploss.(*Stoploss).State == TRIGGERED || stoploss.(*Stoploss).State == EXIT_REJECTED) {
					desk.submitStopExit(symbol, stoploss.(*Stoploss))
				}
			}
		}
	case "balances":
		desk.cacheBalances(artifact)
		desk.retryStopExits()
	case "executions":
		status := orderUpdateStatus(artifact)
		price := datura.Peek[float64](artifact, "last_price")
		side := datura.Peek[string](artifact, "side")
		if symbol == "" {
			symbol = datura.Peek[string](artifact, "data", 0, "symbol")
		}
		if side == "" {
			side = datura.Peek[string](artifact, "data", 0, "side")
		}
		if price <= 0 {
			price = datura.Peek[float64](artifact, "data", 0, "last_price")
		}
		if price <= 0 {
			price = datura.Peek[float64](artifact, "data", 0, "avg_price")
		}
		symbol, side = desk.executionSymbolSide(artifact, symbol, side)

		if terminalExecutionStatus(status) {
			desk.clearPendingForExecution(artifact, status)
		} else if status != "" {
			desk.ackPendingForExecution(artifact, status)
		}

		if terminalFailedExecutionStatus(status) && side == "sell" {
			if stoploss, ok := desk.stoplosses.Load(symbol); ok {
				if typed, typedOK := stoploss.(*Stoploss); typedOK && desk.stoplossExitMatches(typed, artifact) {
					desk.markStopExitRejected(symbol, typed, status)
				}
			}
		}

		desk.recordExecutionFlow(artifact, status)

		if status == "filled" {
			switch side {
			case "buy":
				if price <= 0 {
					return nil
				}

				stoploss := NewStoploss(artifact, symbol)
				if stoploss != nil {
					desk.stoplosses.Store(symbol, stoploss)
					desk.publishStoploss(stoploss)
					if live.Enabled() {
						desk.submitNativeProtectiveStop(symbol, artifact, stoploss)
					}
				}
			case "sell":
				stoploss, ok := desk.stoplosses.Load(symbol)

				if ok {
					if typed, typedOK := stoploss.(*Stoploss); typedOK && typed.order != nil {
						state := stoplossState(typed.order)
						if state != nil {
							state["state"] = int(EXIT_CONFIRMED)
							state["state_label"] = stoplossStateLabel(EXIT_CONFIRMED)
							writeStoplossState(typed.order, state)
						}
					}
					stoploss.(*Stoploss).Close()
					desk.stoplosses.Delete(symbol)
				}
			}
		}
	}

	return nil
}

func terminalExecutionStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "filled", "rejected", "canceled", "cancelled", "expired":
		return true
	default:
		return false
	}
}

func orderUpdateStatus(artifact *datura.Artifact) string {
	if artifact == nil {
		return ""
	}

	for _, path := range [][]any{
		{"order_status"},
		{"status"},
		{"data", 0, "order_status"},
		{"data", 0, "status"},
	} {
		status := datura.Peek[string](artifact, path...)
		if status != "" {
			return status
		}
	}

	return ""
}

func terminalFailedExecutionStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "rejected", "canceled", "cancelled", "expired":
		return true
	default:
		return false
	}
}

func (desk *Desk) storePending(pending *PendingOrder) bool {
	if desk == nil || pending == nil || pending.ClOrdID == "" {
		return false
	}

	ackKey := pendingKey(pending.Symbol, pending.Side)
	ackLocked := false
	if ackKey != ":" && pending.LastStatus == "" {
		if _, exists := desk.pendingAckBySymbolSide.LoadOrStore(ackKey, pending.ClOrdID); exists {
			return false
		}
		ackLocked = true
	}

	if _, exists := desk.pendingByClOrdID.LoadOrStore(pending.ClOrdID, pending); exists {
		if ackLocked {
			desk.releaseAckLock(pending)
		}
		return false
	}

	return true
}

func (desk *Desk) releaseAckLock(pending *PendingOrder) {
	if desk == nil || pending == nil {
		return
	}

	ackKey := pendingKey(pending.Symbol, pending.Side)
	raw, ok := desk.pendingAckBySymbolSide.Load(ackKey)
	if !ok || raw != pending.ClOrdID {
		return
	}

	desk.pendingAckBySymbolSide.Delete(ackKey)
}

func (desk *Desk) clearPendingForExecution(
	artifact *datura.Artifact,
	status string,
) {
	matched := false
	for _, orderID := range executionOrderIDs(artifact) {
		if orderID == "" {
			continue
		}

		if _, ok := desk.orders.Load(orderID); ok {
			desk.orders.Delete(orderID)
		}

		if desk.clearPendingByID(orderID, status) {
			matched = true
		}
	}

	if !matched {
		return
	}
}

func (desk *Desk) ackPendingForExecution(
	artifact *datura.Artifact,
	status string,
) bool {
	if desk == nil || artifact == nil {
		return false
	}

	clOrdID := executionClOrdID(artifact)
	exchangeID := executionExchangeID(artifact)
	if clOrdID == "" && exchangeID == "" {
		return false
	}

	pending := desk.pendingByClientOrExchangeID(clOrdID, exchangeID)
	if pending == nil {
		return false
	}

	if exchangeID != "" && pending.ExchangeOrderID == "" {
		pending.ExchangeOrderID = exchangeID
		desk.pendingByExchangeOrderID.Store(exchangeID, pending)
	}

	pending.LastStatus = status
	desk.releaseAckLock(pending)
	desk.addWorkingOrder(pending)
	desk.markProtectiveWorking(pending)
	return true
}

func (desk *Desk) clearPendingByID(orderID string, status string) bool {
	if desk == nil || orderID == "" {
		return false
	}

	pending := desk.pendingByClientOrExchangeID(orderID, orderID)
	if pending == nil {
		return false
	}

	pending.LastStatus = status
	desk.markProtectiveTerminal(pending, status)
	desk.pendingByClOrdID.Delete(pending.ClOrdID)
	desk.orders.Delete(pending.ClOrdID)
	if pending.ExchangeOrderID != "" {
		desk.pendingByExchangeOrderID.Delete(pending.ExchangeOrderID)
		desk.orders.Delete(pending.ExchangeOrderID)
	}
	desk.releaseAckLock(pending)
	desk.removeWorkingOrder(pending)

	return true
}

func (desk *Desk) pendingByClientOrExchangeID(
	clOrdID string,
	exchangeID string,
) *PendingOrder {
	if desk == nil {
		return nil
	}

	if clOrdID != "" {
		raw, ok := desk.pendingByClOrdID.Load(clOrdID)
		if ok {
			pending, _ := raw.(*PendingOrder)
			return pending
		}
	}

	if exchangeID == "" {
		return nil
	}

	raw, ok := desk.pendingByExchangeOrderID.Load(exchangeID)
	if !ok {
		return nil
	}

	pending, _ := raw.(*PendingOrder)
	return pending
}

func (desk *Desk) addWorkingOrder(pending *PendingOrder) {
	if desk == nil || pending == nil || pending.ClOrdID == "" {
		return
	}

	desk.workingOrdersBySymbol.Store(workingOrderKey(pending.Symbol, pending.ClOrdID), pending)
	if pending.Protective {
		desk.workingProtectiveBySymbol.Store(pending.Symbol, pending)
	}
}

func (desk *Desk) removeWorkingOrder(pending *PendingOrder) {
	if desk == nil || pending == nil {
		return
	}

	desk.workingOrdersBySymbol.Delete(workingOrderKey(pending.Symbol, pending.ClOrdID))
	if !pending.Protective {
		return
	}

	raw, ok := desk.workingProtectiveBySymbol.Load(pending.Symbol)
	if !ok {
		return
	}

	working, _ := raw.(*PendingOrder)
	if working == nil || working.ClOrdID != pending.ClOrdID {
		return
	}

	desk.workingProtectiveBySymbol.Delete(pending.Symbol)
}

func (desk *Desk) markProtectiveWorking(pending *PendingOrder) {
	if desk == nil || pending == nil || !pending.Protective {
		return
	}

	desk.unprotected.Delete(pending.Symbol)
	if pending.Stoploss == nil || pending.Stoploss.order == nil {
		return
	}

	state := stoplossState(pending.Stoploss.order)
	if state == nil {
		return
	}

	state["native_order_id"] = pending.ClOrdID
	if pending.ExchangeOrderID != "" {
		state["native_exchange_order_id"] = pending.ExchangeOrderID
	}
	state["native_state"] = "working"
	writeStoplossState(pending.Stoploss.order, state)
}

func (desk *Desk) markProtectiveTerminal(pending *PendingOrder, status string) {
	if desk == nil || pending == nil || !pending.Protective {
		return
	}

	if terminalFailedExecutionStatus(status) {
		desk.unprotected.Store(pending.Symbol, true)
	}

	if pending.Stoploss == nil || pending.Stoploss.order == nil {
		return
	}

	state := stoplossState(pending.Stoploss.order)
	if state == nil {
		return
	}

	state["native_state"] = status
	state["native_last_status"] = status
	writeStoplossState(pending.Stoploss.order, state)
}

func (desk *Desk) executionSymbolSide(
	artifact *datura.Artifact,
	symbol string,
	side string,
) (string, string) {
	if symbol != "" && side != "" {
		return symbol, side
	}

	for _, orderID := range executionOrderIDs(artifact) {
		raw, ok := desk.orders.Load(orderID)
		if !ok {
			continue
		}

		order, orderOK := raw.(*datura.Artifact)
		if !orderOK {
			continue
		}

		orderSymbol, orderSide := desk.orderSymbolSide(order)
		if symbol == "" {
			symbol = orderSymbol
		}
		if side == "" {
			side = orderSide
		}

		if symbol != "" && side != "" {
			break
		}
	}

	return symbol, side
}

func (desk *Desk) orderSymbolSide(order *datura.Artifact) (string, string) {
	if order == nil {
		return "", ""
	}

	symbol, _ := order.Scope()
	if symbol == "" {
		symbol = datura.Peek[string](order, "symbol")
	}
	if symbol == "" {
		symbol = datura.Peek[string](order, "params", "symbol")
	}

	side := datura.Peek[string](order, "side")
	if side == "" {
		side = datura.Peek[string](order, "params", "side")
	}

	return symbol, side
}

func executionOrderIDs(artifact *datura.Artifact) []string {
	if artifact == nil {
		return nil
	}

	ids := make([]string, 0, 4)
	for _, path := range [][]any{
		{"cl_ord_id"},
		{"order_id"},
		{"data", 0, "cl_ord_id"},
		{"data", 0, "order_id"},
	} {
		if id := datura.Peek[string](artifact, path...); id != "" {
			ids = append(ids, id)
		}
	}

	return ids
}

func executionClOrdID(artifact *datura.Artifact) string {
	if artifact == nil {
		return ""
	}
	if id := datura.Peek[string](artifact, "cl_ord_id"); id != "" {
		return id
	}
	return datura.Peek[string](artifact, "data", 0, "cl_ord_id")
}

func executionExchangeID(artifact *datura.Artifact) string {
	if artifact == nil {
		return ""
	}
	if id := datura.Peek[string](artifact, "order_id"); id != "" {
		return id
	}
	return datura.Peek[string](artifact, "data", 0, "order_id")
}

func (desk *Desk) newOrder(
	symbol string,
	side string,
	orderType string,
	qty float64,
	limitPrice float64,
	triggerPrice float64,
	trailingOffset float64,
) *datura.Artifact {
	order := datura.Acquire(
		"broker", datura.APPJSON,
	).WithDestination(
		"kraken:private",
	).WithRole(
		"orders",
	).WithScope(
		symbol,
	)

	orderID, err := artifactOrderID(order)
	if err != nil {
		errnie.Error(err)
		order.Release()
		return nil
	}

	params := datura.Map[any]{
		"symbol":     symbol,
		"side":       side,
		"order_type": orderType,
		"order_qty":  qty,
		"cl_ord_id":  orderID,
	}

	if orderType == "limit" && limitPrice > 0 {
		params["limit_price"] = limitPrice
	}
	if strings.HasSuffix(orderType, "-limit") && limitPrice > 0 {
		params["limit_price"] = limitPrice
	}
	if triggerPrice > 0 {
		params["trigger_price"] = triggerPrice
	}
	if trailingOffset > 0 {
		params["trailing_stop"] = trailingOffset
	}

	order.WithPayload(datura.Map[any]{
		"method": "add_order",
		"params": params,
	}.Marshal())

	return order
}

func (desk *Desk) sendPrivate(order *datura.Artifact) {
	if order == nil {
		return
	}
	desk.recordPrivateSubmission(order)

	bg, ok := desk.broadcasts.Load("kraken:private")
	if !ok {
		return
	}

	bg.(*qpool.BroadcastGroup).Send(order)
}

func (desk *Desk) resolveOrderQuantity(
	action *datura.Artifact,
	actionType string,
	side string,
	symbol string,
) float64 {
	if qty := datura.Peek[float64](action, "quantity"); qty > 0 {
		return qty
	}

	base := baseAsset(symbol)

	if strings.EqualFold(side, "sell") || actionType == "settle_position" {
		return desk.balanceForAsset(base)
	}

	fraction := datura.Peek[float64](action, "fraction")
	if fraction <= 0 {
		return 0
	}

	price := desk.actionPrice(action, side, symbol)
	if price <= 0 {
		return 0
	}

	cash := desk.balanceForAsset(desk.quote)
	if cash <= 0 {
		return 0
	}

	return (cash * fraction) / price
}

func (desk *Desk) actionPrice(action *datura.Artifact, side string, symbol string) float64 {
	for _, path := range [][]any{
		{"price"},
		{"limit_price"},
		{"params", "limit_price"},
		{"params", "price"},
	} {
		price := datura.Peek[float64](action, path...)
		if price > 0 {
			return price
		}
	}

	if quote, ok := desk.quoteForSymbol(symbol); ok {
		switch strings.ToLower(side) {
		case "buy":
			if quote.ask > 0 {
				return quote.ask
			}
		case "sell":
			if quote.bid > 0 {
				return quote.bid
			}
		}

		if quote.last > 0 {
			return quote.last
		}
	}

	if mark, ok := desk.marks.Load(symbol); ok {
		if price, priceOK := mark.(float64); priceOK && price > 0 {
			return price
		}
	}

	return 0
}

func (desk *Desk) limitPrice(action *datura.Artifact, side string, symbol string) float64 {
	for _, path := range [][]any{
		{"limit_price"},
		{"params", "limit_price"},
		{"price"},
		{"params", "price"},
	} {
		price := datura.Peek[float64](action, path...)
		if price > 0 {
			return price
		}
	}

	if quote, ok := desk.quoteForSymbol(symbol); ok {
		switch strings.ToLower(side) {
		case "buy":
			if quote.bid > 0 {
				return quote.bid
			}
		case "sell":
			if quote.ask > 0 {
				return quote.ask
			}
		}

		if quote.last > 0 {
			return quote.last
		}
	}

	if mark, ok := desk.marks.Load(symbol); ok {
		if price, priceOK := mark.(float64); priceOK && price > 0 {
			return price
		}
	}

	return 0
}

func (desk *Desk) triggerPrice(action *datura.Artifact, side string, symbol string) float64 {
	for _, path := range [][]any{
		{"trigger_price"},
		{"params", "trigger_price"},
		{"price"},
		{"params", "price"},
		{"stop"},
		{"params", "stop"},
	} {
		price := datura.Peek[float64](action, path...)
		if price > 0 {
			return price
		}
	}

	return desk.limitPrice(action, side, symbol)
}

func requiresTriggerPrice(orderType string) bool {
	switch orderType {
	case "stop-loss", "stop-loss-limit", "take-profit", "take-profit-limit":
		return true
	default:
		return false
	}
}

func requiresTrailingOffset(orderType string) bool {
	return orderType == "trailing-stop" || orderType == "trailing-stop-limit"
}

func trailingOffsetForAction(action *datura.Artifact, orderType string) float64 {
	if !requiresTrailingOffset(orderType) {
		return 0
	}
	for _, path := range [][]any{
		{"trailing_stop"},
		{"params", "trailing_stop"},
		{"offset"},
		{"params", "offset"},
	} {
		offset := datura.Peek[float64](action, path...)
		if offset > 0 {
			return offset
		}
	}

	return 0
}

type marketQuote struct {
	bid  float64
	ask  float64
	last float64
}

func (desk *Desk) quoteForSymbol(symbol string) (marketQuote, bool) {
	raw, ok := desk.quotes.Load(symbol)
	if !ok {
		return marketQuote{}, false
	}

	quote, quoteOK := raw.(marketQuote)
	return quote, quoteOK
}

func (desk *Desk) balanceForAsset(asset string) float64 {
	if desk == nil || asset == "" {
		return 0
	}

	target := strings.ToUpper(asset)

	desk.balanceMu.RLock()
	defer desk.balanceMu.RUnlock()

	return desk.balances[target]
}

type deskBalanceFrame struct {
	Data  []deskBalanceRow `json:"data"`
	Asset []deskBalanceRow `json:"asset"`
}

type deskBalanceRow struct {
	Asset   string  `json:"asset"`
	Balance float64 `json:"balance"`
}

func (desk *Desk) cacheBalances(artifact *datura.Artifact) {
	if desk == nil || artifact == nil {
		return
	}

	var frame deskBalanceFrame
	if err := sonic.Unmarshal(artifact.DecryptPayload(), &frame); err != nil {
		return
	}

	next := make(map[string]float64, len(frame.Data)+len(frame.Asset))
	for _, row := range append(frame.Data, frame.Asset...) {
		asset := strings.ToUpper(strings.TrimSpace(row.Asset))
		if asset == "" {
			continue
		}

		next[asset] = row.Balance
	}

	if len(next) == 0 {
		return
	}

	desk.balanceMu.Lock()
	desk.balances = next
	desk.balanceMu.Unlock()
}

func (desk *Desk) retryStopExits() {
	if desk == nil {
		return
	}

	desk.stoplosses.Range(func(key any, value any) bool {
		symbol, _ := key.(string)
		stoploss, _ := value.(*Stoploss)
		if symbol == "" || stoploss == nil ||
			(stoploss.State != TRIGGERED && stoploss.State != EXIT_REJECTED) {
			return true
		}
		if live.Enabled() {
			return true
		}

		desk.submitStopExit(symbol, stoploss)
		return true
	})
}

func (desk *Desk) stoplossExitMatches(stoploss *Stoploss, artifact *datura.Artifact) bool {
	if stoploss == nil || stoploss.order == nil || artifact == nil {
		return false
	}

	state := stoplossState(stoploss.order)
	exitID := stoplossString(state, "exit_order_id")
	if exitID == "" {
		return false
	}

	for _, orderID := range executionOrderIDs(artifact) {
		if orderID == exitID {
			return true
		}
	}

	return false
}

func (desk *Desk) markStopExitRejected(symbol string, stoploss *Stoploss, status string) {
	if stoploss == nil || stoploss.order == nil {
		return
	}

	state := stoplossState(stoploss.order)
	if state == nil {
		return
	}

	retries := stoplossInt(state, "retry_count") + 1
	state["exit_order_id"] = ""
	state["retry_count"] = retries
	state["last_error"] = status
	state["state"] = int(EXIT_REJECTED)
	state["state_label"] = stoplossStateLabel(EXIT_REJECTED)
	writeStoplossState(stoploss.order, state)
	stoploss.State = EXIT_REJECTED

	if retries >= 3 {
		desk.entryBlocked.Store(true)
		desk.publishCriticalStopDiagnostic(symbol)
	}
}

func baseAsset(symbol string) string {
	base, _, _ := strings.Cut(symbol, "/")
	return strings.ToUpper(strings.TrimSpace(base))
}

func (desk *Desk) symbolOpen(symbol string) bool {
	if desk == nil || symbol == "" {
		return false
	}
	if _, ok := desk.stoplosses.Load(symbol); ok {
		return true
	}
	return desk.balanceForAsset(baseAsset(symbol)) > 0
}

func (desk *Desk) nativeProtectionMissing(symbol string) bool {
	if desk == nil || symbol == "" {
		return false
	}

	_, missing := desk.unprotected.Load(symbol)
	return missing
}

func (desk *Desk) submitNativeProtectiveStop(
	symbol string,
	fill *datura.Artifact,
	stoploss *Stoploss,
) bool {
	if desk == nil || !live.Enabled() {
		return false
	}
	desk.unprotected.Store(symbol, true)
	if !live.NativeProtectiveStopsSupported() {
		desk.publishDiagnosticPayload(datura.Map[any]{
			"severity": "critical",
			"symbol":   symbol,
			"side":     "sell",
			"reason":   "native_stop_missing",
		})
		return false
	}

	qty := executionQuantity(fill)
	if qty <= 0 {
		qty = desk.balanceForAsset(baseAsset(symbol))
	}
	if qty <= 0 {
		desk.publishDiagnosticPayload(datura.Map[any]{
			"severity": "critical",
			"symbol":   symbol,
			"side":     "sell",
			"reason":   "native_stop_missing",
		})
		return false
	}

	trailingOffset := viper.GetFloat64("trading.stop.trailing_offset_bps")
	if trailingOffset <= 0 {
		desk.publishDiagnosticPayload(datura.Map[any]{
			"severity": "critical",
			"symbol":   symbol,
			"side":     "sell",
			"reason":   "native_stop_missing",
		})
		return false
	}

	order := desk.newOrder(
		symbol,
		"sell",
		string(logic.OrderTrailingStop),
		qty,
		0,
		0,
		trailingOffset,
	)
	if order == nil {
		return false
	}

	orderID, err := artifactOrderID(order)
	if err != nil {
		errnie.Error(err)
		order.Release()
		return false
	}

	pending := &PendingOrder{
		ClOrdID:    orderID,
		Symbol:     symbol,
		Side:       "sell",
		OrderType:  string(logic.OrderTrailingStop),
		Qty:        qty,
		CreatedAt:  time.Now().UTC(),
		Protective: true,
		Stoploss:   stoploss,
		Attempt:    1,
	}
	if !desk.storePending(pending) {
		order.Release()
		return false
	}

	if stoploss != nil && stoploss.order != nil {
		state := stoplossState(stoploss.order)
		if state != nil {
			state["native_order_id"] = orderID
			state["native_state"] = "submitted"
			writeStoplossState(stoploss.order, state)
		}
	}

	desk.orders.Store(orderID, order)
	desk.sendPrivate(order)
	return true
}

func (desk *Desk) submitStopExit(symbol string, stoploss *Stoploss) {
	if stoploss == nil || stoploss.order == nil {
		return
	}

	state := stoplossState(stoploss.order)
	if state == nil {
		return
	}

	if stoplossString(state, "exit_order_id") != "" {
		return
	}

	if stoplossInt(state, "retry_count") >= 3 {
		desk.entryBlocked.Store(true)
		desk.publishCriticalStopDiagnostic(symbol)
		return
	}

	qty := desk.balanceForAsset(baseAsset(symbol))
	if qty <= 0 {
		if stoplossString(state, "waiting_balance") == "" {
			errnie.Warn("broker: stop exit waiting for held quantity", "symbol", symbol)
		}
		state["waiting_balance"] = time.Now().UTC().Format(time.RFC3339Nano)
		writeStoplossState(stoploss.order, state)
		return
	}

	order := desk.newOrder(symbol, "sell", "market", qty, 0, 0, 0)
	if order == nil {
		return
	}

	orderID, err := artifactOrderID(order)
	if err != nil {
		errnie.Error(err)
		order.Release()
		return
	}

	pending := &PendingOrder{
		ClOrdID:    orderID,
		Symbol:     symbol,
		Side:       "sell",
		OrderType:  "market",
		Qty:        qty,
		CreatedAt:  time.Now().UTC(),
		Protective: true,
		Stoploss:   stoploss,
		Attempt:    stoplossInt(state, "retry_count") + 1,
	}
	if !desk.storePending(pending) {
		order.Release()
		return
	}

	state["waiting_balance"] = ""
	state["exit_order_id"] = orderID
	state["state"] = int(EXIT_SUBMITTED)
	writeStoplossState(stoploss.order, state)
	stoploss.State = EXIT_SUBMITTED
	desk.orders.Store(orderID, order)
	desk.sendPrivate(order)
}

func executionQuantity(artifact *datura.Artifact) float64 {
	for _, path := range [][]any{
		{"order_qty"},
		{"last_qty"},
		{"cum_qty"},
		{"data", 0, "order_qty"},
		{"data", 0, "last_qty"},
		{"data", 0, "cum_qty"},
	} {
		qty := datura.Peek[float64](artifact, path...)
		if qty > 0 {
			return qty
		}
	}

	return 0
}

func (desk *Desk) publishCriticalStopDiagnostic(symbol string) {
	desk.publishDiagnosticPayload(datura.Map[any]{
		"severity": "critical",
		"symbol":   symbol,
		"reason":   "stop_exit_retry_exhausted",
	})
}

func (desk *Desk) publishDiagnostic(action *datura.Artifact, severity string, reason string) {
	if action == nil {
		return
	}

	symbol, _ := action.Scope()
	desk.publishDiagnosticPayload(datura.Map[any]{
		"severity":            severity,
		"symbol":              symbol,
		"side":                datura.Peek[string](action, "side"),
		"order_type":          datura.Peek[string](action, "type"),
		"reason":              reason,
		"score":               datura.Peek[float64](action, "decision", "score"),
		"expected_return_bps": datura.Peek[float64](action, "decision", "expected_return_bps"),
		"friction_bps":        datura.Peek[float64](action, "decision", "friction_bps"),
		"net_edge_bps":        datura.Peek[float64](action, "decision", "net_edge_bps"),
		"notional":            datura.Peek[float64](action, "notional"),
		"available_quote":     desk.balanceForAsset(desk.quote),
		"position_count":      desk.positionCount(),
		"pending_count":       desk.pendingCount(),
	})
}

func diagnosticReason(action *datura.Artifact) string {
	if action == nil {
		return ""
	}
	if reason := datura.Peek[string](action, "risk", "reason"); reason != "" {
		return reason
	}
	if reason := datura.Peek[string](action, "why"); reason != "" {
		return reason
	}
	if strings.EqualFold(datura.Peek[string](action, "side"), "buy") &&
		!datura.Peek[bool](action, "risk", "stamped") {
		return "allocator_stamp_missing"
	}
	return datura.Peek[string](action, "decision", "reason")
}

func (desk *Desk) publishPendingDiagnostic(
	pending *PendingOrder,
	severity string,
	reason string,
) {
	if pending == nil {
		return
	}

	desk.publishDiagnosticPayload(datura.Map[any]{
		"severity":        severity,
		"symbol":          pending.Symbol,
		"side":            pending.Side,
		"order_type":      pending.OrderType,
		"reason":          reason,
		"notional":        pending.Notional,
		"available_quote": desk.balanceForAsset(desk.quote),
		"position_count":  desk.positionCount(),
		"pending_count":   desk.pendingCount(),
	})
}

func (desk *Desk) publishDiagnosticPayload(payload datura.Map[any]) {
	if payload == nil {
		return
	}

	symbol, _ := payload["symbol"].(string)
	if symbol == "" {
		symbol = "broker"
	}

	payload["channel"] = "broker"
	payload["type"] = "diagnostic"
	payload["timestamp"] = time.Now().UTC().Format(time.RFC3339Nano)

	artifact := datura.Acquire("broker", datura.APPJSON).
		WithRole("diagnostic").
		WithScope(symbol).
		WithDestination("ui").
		WithPayload(payload.Marshal())

	if desk.pool != nil {
		desk.pool.CreateBroadcastGroup("ui").Send(artifact)
	}
	if desk.tree != nil {
		if updated, _, err := desk.tree.InsertArtifact(artifact.Prefix("role", "scope", "timestamp"), artifact); err != nil {
			errnie.Error(err)
		} else if updated != nil {
			desk.tree = updated
		}
	}
}

func (desk *Desk) positionCount() int {
	if desk == nil {
		return 0
	}

	desk.balanceMu.RLock()
	defer desk.balanceMu.RUnlock()

	count := 0
	for asset, qty := range desk.balances {
		if qty <= 0 || strings.EqualFold(asset, desk.quote) {
			continue
		}
		count++
	}

	return count
}

func (desk *Desk) pendingCount() int {
	if desk == nil || desk.pending == nil {
		return 0
	}

	count := 0
	desk.pending.Range(func(_ any, value any) bool {
		if pending, ok := value.(*PendingOrder); ok && pending != nil {
			count++
		}
		return true
	})

	return count
}

func stoplossString(state map[string]any, key string) string {
	value, _ := state[key].(string)
	return value
}

func stoplossInt(state map[string]any, key string) int {
	switch value := state[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func artifactOrderID(order *datura.Artifact) (string, error) {
	uuid, err := order.Uuid()
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(uuid), nil
}

func (desk *Desk) publishStoploss(stoploss *Stoploss) {
	if desk == nil || stoploss == nil || stoploss.order == nil {
		return
	}

	state := stoplossState(stoploss.order)
	if state == nil {
		return
	}

	artifact := datura.Acquire("broker", datura.APPJSON).
		WithRole("stoploss").
		WithScope(stoploss.Symbol).
		WithDestination("ui")
	artifact.SetTimestamp(time.Now().UTC().UnixNano())
	artifact.WithPayload(datura.Map[any]{
		"symbol":          stoploss.Symbol,
		"state":           state["state"],
		"state_label":     state["state_label"],
		"last_mark":       state["last_mark"],
		"recent_marks":    state["recent_marks"],
		"peak":            state["peak"],
		"stop":            state["stop"],
		"offset":          state["offset"],
		"side":            state["side"],
		"trigger":         state["trigger"],
		"triggered_at":    state["triggered_at"],
		"exit_order_id":   state["exit_order_id"],
		"retry_count":     state["retry_count"],
		"last_error":      state["last_error"],
		"waiting_balance": state["waiting_balance"],
	}.Marshal())

	if desk.tree != nil {
		if updated, _, err := desk.tree.InsertArtifact(artifact.Prefix("role", "scope", "timestamp"), artifact); err != nil {
			errnie.Error(err)
		} else if updated != nil {
			desk.tree = updated
		}
	}

	if desk.pool != nil {
		desk.pool.CreateBroadcastGroup("ui").Send(artifact)
	}
}

func (desk *Desk) Close() error {
	desk.closed.Store(true)
	desk.cancel()
	return nil
}

func (desk *Desk) PendingEntryCount() int {
	if desk == nil || desk.pending == nil {
		return 0
	}

	count := 0
	desk.pending.Range(func(_ any, value any) bool {
		pending, ok := value.(*PendingOrder)
		if !ok || pending == nil {
			return true
		}

		if pendingLiveEntryExposure(pending) {
			count++
		}

		return true
	})

	return count
}
