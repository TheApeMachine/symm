package broker

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
)

type Position struct {
	mu          sync.RWMutex
	id          string
	symbol      string
	openedAt    time.Time
	closedAt    *time.Time
	events      []LifeEvent
	entryOrders []*spot.OrderRequest
	exitOrders  []*spot.OrderRequest
	fills       []*kraken.ExecutionData
	qty         *decimal.Decimal
	entryPrice  *decimal.Decimal
	exitPrice   *decimal.Decimal
	entryFee    *decimal.Decimal
	exitFee     *decimal.Decimal
	mark        *decimal.Decimal
	outcome     *LifeOutcome
}

func NewPosition() *Position {
	return &Position{
		id:          uuid.NewString(),
		events:      make([]LifeEvent, 0),
		entryOrders: make([]*spot.OrderRequest, 0),
		exitOrders:  make([]*spot.OrderRequest, 0),
		fills:       make([]*kraken.ExecutionData, 0),
	}
}

/*
RestorePosition recreates a historically closed position loaded from Iceberg.
*/
func RestorePosition(
	id string,
	symbol string,
	openedAt time.Time,
	closedAt *time.Time,
	events []LifeEvent,
	outcome *LifeOutcome,
) *Position {
	return &Position{
		id:          id,
		symbol:      symbol,
		openedAt:    openedAt,
		closedAt:    closedAt,
		events:      events,
		outcome:     outcome,
		entryOrders: make([]*spot.OrderRequest, 0),
		exitOrders:  make([]*spot.OrderRequest, 0),
		fills:       make([]*kraken.ExecutionData, 0),
	}
}

/*
Enter records a buy of qty and returns the order to submit.
*/
func (position *Position) Enter(symbol string, qty *decimal.Decimal, path ...string) *spot.OrderRequest {
	position.mu.Lock()
	defer position.mu.Unlock()

	position.qty = qty
	position.symbol = symbol

	if position.openedAt.IsZero() {
		position.openedAt = time.Now().UTC()
	}

	if len(path) > 0 && path[0] != "" {
		position.events = append(position.events, LifeEvent{
			At:     position.openedAt.Format(time.RFC3339),
			Kind:   "entry_match",
			Detail: fmt.Sprintf("prefix match %s", path[0]),
			Fields: map[string]any{
				"path":   path[0],
				"symbol": symbol,
			},
		})
	}

	clOrdId := uuid.NewString()

	order := &spot.OrderRequest{
		ClOrdId:   clOrdId,
		OrderType: "market",
		Type:      "buy",
		Volume:    qty.String(),
	}

	position.entryOrders = append(position.entryOrders, order)

	position.events = append(position.events, LifeEvent{
		At:     position.openedAt.Format(time.RFC3339),
		Kind:   "entry_order",
		Detail: fmt.Sprintf("order buy %s %s", qty.String(), symbol),
		Fields: map[string]any{
			"cl_ord_id": clOrdId,
			"symbol":    symbol,
			"volume":    qty.String(),
		},
	})

	return order
}

/*
Exit records a sell of everything open and returns the order to submit. A
position that is closed or already has its sell pending returns nil: selling
it again would sell quantity it no longer holds.
*/
func (position *Position) Exit(symbol string) *spot.OrderRequest {
	position.mu.Lock()
	defer position.mu.Unlock()

	if position.closedLocked() || len(position.exitOrders) > 0 {
		return nil
	}

	volume := position.qty.String()
	clOrdId := uuid.NewString()

	order := &spot.OrderRequest{
		ClOrdId:   clOrdId,
		OrderType: "market",
		Type:      "sell",
		Volume:    volume,
	}

	position.exitOrders = append(position.exitOrders, order)

	position.events = append(position.events, LifeEvent{
		At:     time.Now().UTC().Format(time.RFC3339),
		Kind:   "exit_order",
		Detail: fmt.Sprintf("order sell %s %s", volume, symbol),
		Fields: map[string]any{
			"cl_ord_id": clOrdId,
			"symbol":    symbol,
			"volume":    volume,
		},
	})

	return order
}

/*
abandonExit drops the sell Exit just recorded when the venue did not take it,
so a later exit can submit again.
*/
func (position *Position) abandonExit() {
	position.mu.Lock()
	defer position.mu.Unlock()

	if len(position.exitOrders) > 0 {
		position.exitOrders = position.exitOrders[:len(position.exitOrders)-1]
	}

	if n := len(position.events); n > 0 && position.events[n-1].Kind == "exit_order" {
		position.events = position.events[:n-1]
	}
}

func (position *Position) Qty() *decimal.Decimal {
	position.mu.RLock()
	defer position.mu.RUnlock()

	return position.qty
}

func (position *Position) EntryPrice() *decimal.Decimal {
	position.mu.RLock()
	defer position.mu.RUnlock()

	return position.entryPrice
}

func (position *Position) ExitPrice() *decimal.Decimal {
	position.mu.RLock()
	defer position.mu.RUnlock()

	return position.exitPrice
}

func (position *Position) closedLocked() bool {
	return position.qty == nil || position.qty.Sign() <= 0
}

func (position *Position) Closed() bool {
	position.mu.RLock()
	defer position.mu.RUnlock()

	return position.closedLocked()
}

func (position *Position) Lifecycle() Lifecycle {
	position.mu.RLock()
	defer position.mu.RUnlock()

	status := "open"

	if position.closedLocked() {
		status = "closed"
	}

	openedAt := ""

	if !position.openedAt.IsZero() {
		openedAt = position.openedAt.Format(time.RFC3339)
	}

	closedAt := ""

	if position.closedAt != nil {
		closedAt = position.closedAt.Format(time.RFC3339)
	}

	lifecycle := Lifecycle{
		Id:       position.id,
		Symbol:   position.symbol,
		Status:   status,
		OpenedAt: openedAt,
		ClosedAt: closedAt,
		Events:   position.events,
	}

	if position.outcome != nil {
		lifecycle.Outcome = position.outcome
		return lifecycle
	}

	if position.closedLocked() && len(position.fills) > 0 {
		outcome := &LifeOutcome{
			ShadowDefined: false,
			Triggers:      []string{},
		}

		var cost float64
		var proceeds float64
		var fees float64

		for _, fill := range position.fills {
			if fill.FeeUsdEquiv != nil {
				fees += fill.FeeUsdEquiv.Float64()
			}

			if fill.LastQty == nil || fill.LastPrice == nil {
				continue
			}

			val := fill.LastQty.Float64() * fill.LastPrice.Float64()

			if strings.ToLower(fill.Side) == "buy" {
				cost += val
			}

			if strings.ToLower(fill.Side) == "sell" {
				proceeds += val
			}
		}

		outcome.VenueCost = cost
		outcome.VenueProceeds = proceeds
		outcome.VenueFees = fees
		outcome.VenueRealized = proceeds - cost - fees

		if position.closedAt != nil && !position.openedAt.IsZero() {
			outcome.HoldNs = position.closedAt.Sub(position.openedAt).Nanoseconds()
		}

		if position.closedAt == nil && !position.openedAt.IsZero() {
			outcome.HoldNs = time.Since(position.openedAt).Nanoseconds()
		}

		lifecycle.Outcome = outcome
	}

	return lifecycle
}

func (position *Position) ID() string {
	position.mu.RLock()
	defer position.mu.RUnlock()

	return position.id
}

func (position *Position) Symbol() string {
	position.mu.RLock()
	defer position.mu.RUnlock()

	return position.symbol
}

func (position *Position) ClosedAt() *time.Time {
	position.mu.RLock()
	defer position.mu.RUnlock()

	return position.closedAt
}

func (position *Position) Events() []LifeEvent {
	position.mu.RLock()
	defer position.mu.RUnlock()

	return slices.Clone(position.events)
}

func (position *Position) Apply(execution kraken.ExecutionData) {
	position.mu.Lock()
	defer position.mu.Unlock()

	position.fills = append(position.fills, &execution)

	detail := fmt.Sprintf("%s fill", execution.Side)

	if execution.LastQty != nil && execution.LastPrice != nil {
		detail = fmt.Sprintf("%s fill %s @ %s", execution.Side, execution.LastQty.String(), execution.LastPrice.String())
	}

	fields := map[string]any{
		"side":      execution.Side,
		"exec_type": execution.ExecType,
	}

	if execution.LastQty != nil {
		fields["volume"] = execution.LastQty.String()
	}

	if execution.LastPrice != nil {
		fields["price"] = execution.LastPrice.String()
	}

	if execution.LastQty != nil && execution.LastPrice != nil {
		fields["cost"] = execution.LastQty.Mul(execution.LastPrice).String()
	}

	if execution.FeeUsdEquiv != nil {
		fields["fee"] = execution.FeeUsdEquiv.String()
	}

	if execution.AvgPrice != nil {
		fields["avg_price"] = execution.AvgPrice.String()
	}

	event := LifeEvent{
		At:     time.Now().UTC().Format(time.RFC3339),
		Kind:   "fill",
		Detail: detail,
		Fields: fields,
	}

	if !execution.Timestamp.IsZero() {
		event.VenueAt = execution.Timestamp.UTC().Format(time.RFC3339)
	}

	position.events = append(position.events, event)

	if execution.LastQty == nil || execution.LastQty.Sign() <= 0 {
		return
	}

	side := strings.ToLower(execution.Side)

	if side == "buy" {
		if execution.CumQty != nil && execution.CumQty.Sign() > 0 {
			position.qty = execution.CumQty
		}

		if execution.CumQty == nil || execution.CumQty.Sign() <= 0 {
			if position.qty == nil {
				position.qty = execution.LastQty
			}

			if position.qty != nil && position.qty != execution.LastQty {
				position.qty = position.qty.Add(execution.LastQty)
			}
		}

		if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
			position.entryPrice = execution.AvgPrice
		}

		if (execution.AvgPrice == nil || execution.AvgPrice.Sign() <= 0) && execution.LastPrice != nil {
			position.entryPrice = execution.LastPrice
		}

		if execution.FeeUsdEquiv != nil {
			if position.entryFee == nil {
				position.entryFee = execution.FeeUsdEquiv
			}

			if position.entryFee != nil && position.entryFee != execution.FeeUsdEquiv {
				position.entryFee = position.entryFee.Add(execution.FeeUsdEquiv)
			}
		}

		return
	}

	if side == "sell" {
		if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
			position.exitPrice = execution.AvgPrice
		}

		if (execution.AvgPrice == nil || execution.AvgPrice.Sign() <= 0) && execution.LastPrice != nil {
			position.exitPrice = execution.LastPrice
		}

		if execution.FeeUsdEquiv != nil {
			if position.exitFee == nil {
				position.exitFee = execution.FeeUsdEquiv
			}

			if position.exitFee != nil && position.exitFee != execution.FeeUsdEquiv {
				position.exitFee = position.exitFee.Add(execution.FeeUsdEquiv)
			}
		}

		if position.qty != nil {
			position.qty = position.qty.Sub(execution.LastQty)

			if position.qty.Sign() <= 0 {
				position.qty = decimal.NewFromFloat64(0)
			}
		}
	}

	if position.closedLocked() && position.closedAt == nil {
		now := time.Now().UTC()
		position.closedAt = &now
	}
}

/*
OpenedAt returns when the position's first entry was recorded (zero if never).
*/
func (position *Position) OpenedAt() time.Time {
	position.mu.RLock()
	defer position.mu.RUnlock()

	return position.openedAt
}

/*
EntryFee returns the venue fee paid on entry fills so far, nil before any.
*/
func (position *Position) EntryFee() *decimal.Decimal {
	position.mu.RLock()
	defer position.mu.RUnlock()

	return position.entryFee
}

/*
Exiting reports whether a sell for the position has been submitted.
*/
func (position *Position) Exiting() bool {
	position.mu.RLock()
	defer position.mu.RUnlock()

	return len(position.exitOrders) > 0
}

/*
MarshalJSON exposes the position's lifecycle and economic state, so a tick
Value can carry it as a layer.
*/
func (position *Position) MarshalJSON() ([]byte, error) {
	lifecycle := position.Lifecycle()

	position.mu.RLock()
	defer position.mu.RUnlock()

	return sonic.Marshal(struct {
		Lifecycle
		Qty        *decimal.Decimal `json:"qty,omitempty"`
		EntryPrice *decimal.Decimal `json:"entry_price,omitempty"`
		ExitPrice  *decimal.Decimal `json:"exit_price,omitempty"`
		EntryFee   *decimal.Decimal `json:"entry_fee,omitempty"`
		ExitFee    *decimal.Decimal `json:"exit_fee,omitempty"`
		Mark       *decimal.Decimal `json:"mark,omitempty"`
	}{
		Lifecycle:  lifecycle,
		Qty:        position.qty,
		EntryPrice: position.entryPrice,
		ExitPrice:  position.exitPrice,
		EntryFee:   position.entryFee,
		ExitFee:    position.exitFee,
		Mark:       position.mark,
	})
}
