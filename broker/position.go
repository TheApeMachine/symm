package broker

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
)

type Position struct {
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

func (position *Position) Enter(symbol string, qty *decimal.Decimal) error {
	position.qty = qty
	position.symbol = symbol

	if position.openedAt.IsZero() {
		position.openedAt = time.Now().UTC()
	}

	clOrdId := uuid.NewString()

	position.entryOrders = append(position.entryOrders, &spot.OrderRequest{
		ClOrdId:   clOrdId,
		OrderType: "limit",
		Type:      "buy",
		Volume:    qty.String(),
	})

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

	return nil
}

func (position *Position) Exit(symbol string) error {
	volume := "0"

	if position.qty != nil {
		volume = position.qty.String()
	}

	clOrdId := uuid.NewString()

	position.exitOrders = append(position.exitOrders, &spot.OrderRequest{
		ClOrdId:   clOrdId,
		OrderType: "limit",
		Type:      "sell",
		Volume:    volume,
	})

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

	return nil
}

func (position *Position) Qty() *decimal.Decimal {
	return position.qty
}

func (position *Position) EntryPrice() *decimal.Decimal {
	return position.entryPrice
}

func (position *Position) ExitPrice() *decimal.Decimal {
	return position.exitPrice
}

func (position *Position) Closed() bool {
	return position.qty == nil || position.qty.Sign() <= 0
}

func (position *Position) Lifecycle() Lifecycle {
	status := "open"

	if position.Closed() {
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

	if len(position.fills) > 0 || position.Closed() {
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

func (position *Position) Apply(execution kraken.ExecutionData) {
	position.fills = append(position.fills, &execution)

	detail := fmt.Sprintf("%s fill", execution.Side)

	if execution.LastQty != nil && execution.LastPrice != nil {
		detail = fmt.Sprintf("%s fill %s @ %s", execution.Side, execution.LastQty.String(), execution.LastPrice.String())
	}

	event := LifeEvent{
		At:     time.Now().UTC().Format(time.RFC3339),
		Kind:   "fill",
		Detail: detail,
		Fields: map[string]any{
			"side":      execution.Side,
			"exec_type": execution.ExecType,
		},
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

	if position.Closed() && position.closedAt == nil {
		now := time.Now().UTC()
		position.closedAt = &now
	}
}
