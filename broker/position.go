package broker

import (
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
)

type Position struct {
	mu          sync.RWMutex
	ID          string                  `json:"id"`
	Symbol      string                  `json:"symbol"`
	OpenedAt    time.Time               `json:"opened_at"`
	ClosedAt    *time.Time              `json:"closed_at"`
	EntryOrders []*spot.OrderRequest    `json:"entry_orders,omitempty"`
	ExitOrders  []*spot.OrderRequest    `json:"exit_orders,omitempty"`
	Fills       []*kraken.ExecutionData `json:"fills,omitempty"`
	Qty         *decimal.Decimal        `json:"qty,omitempty"`
	EntryPrice  *decimal.Decimal        `json:"entry_price,omitempty"`
	ExitPrice   *decimal.Decimal        `json:"exit_price,omitempty"`
	EntryFee    *decimal.Decimal        `json:"entry_fee,omitempty"`
	ExitFee     *decimal.Decimal        `json:"exit_fee,omitempty"`
	Mark        *decimal.Decimal        `json:"mark,omitempty"`
	ExitVWAP    *decimal.Decimal        `json:"exit_vwap,omitempty"`
	ExitRoom    *float64                `json:"exit_room,omitempty"`
	Return      *float64                `json:"return,omitempty"`
	HoldNs      int64                   `json:"hold_ns,omitempty"`
	Virtual     bool                    `json:"virtual,omitempty"`
}

func NewPosition() *Position {
	return &Position{
		ID:          uuid.NewString(),
		EntryOrders: make([]*spot.OrderRequest, 0),
		ExitOrders:  make([]*spot.OrderRequest, 0),
		Fills:       make([]*kraken.ExecutionData, 0),
	}
}

/*
Enter records a buy of qty and returns the order to submit.
*/
func (position *Position) Enter(
	symbol string, qty *decimal.Decimal, path ...string,
) *spot.OrderRequest {
	position.mu.Lock()
	defer position.mu.Unlock()

	position.Qty = qty
	position.Symbol = symbol

	if position.OpenedAt.IsZero() {
		position.OpenedAt = time.Now().UTC()
	}

	if len(path) > 0 && path[0] != "" {
		position.EntryOrders = append(position.EntryOrders, &spot.OrderRequest{
			ClOrdId:   position.ID,
			OrderType: "market",
			Type:      "buy",
			Volume:    qty.String(),
		})
	}

	clOrdId := uuid.NewString()

	order := &spot.OrderRequest{
		ClOrdId:   clOrdId,
		OrderType: "market",
		Type:      "buy",
		Volume:    qty.String(),
	}

	position.EntryOrders = append(position.EntryOrders, order)
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

	if position.ClosedAt != nil && position.ClosedAt.IsZero() || len(position.ExitOrders) > 0 {
		return nil
	}

	volume := position.Qty.String()
	clOrdId := uuid.NewString()

	order := &spot.OrderRequest{
		ClOrdId:   clOrdId,
		OrderType: "market",
		Type:      "sell",
		Volume:    volume,
	}

	position.ExitOrders = append(position.ExitOrders, order)
	return order
}

func (position *Position) Lifecycle() *Position {
	position.mu.RLock()
	defer position.mu.RUnlock()

	if position.ClosedAt != nil && position.ClosedAt.IsZero() && len(position.Fills) > 0 {
		for _, fill := range position.Fills {
			if fill.FeeUsdEquiv != nil {
				position.ExitFee = position.ExitFee.Add(fill.FeeUsdEquiv)
			}
		}

		if position.ClosedAt != nil && !position.OpenedAt.IsZero() {
			position.HoldNs = position.ClosedAt.Sub(position.OpenedAt).Nanoseconds()
		}

		if position.ClosedAt == nil && !position.OpenedAt.IsZero() {
			position.HoldNs = time.Since(position.OpenedAt).Nanoseconds()
		}
	}

	return position
}

func (position *Position) Apply(execution kraken.ExecutionData) {
	position.mu.Lock()
	defer position.mu.Unlock()

	position.Fills = append(position.Fills, &execution)

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

	if !execution.Timestamp.IsZero() {
		fields["venue_at"] = execution.Timestamp.UTC().Format(time.RFC3339)
	}

	if execution.LastQty == nil || execution.LastQty.Sign() <= 0 {
		return
	}

	side := strings.ToLower(execution.Side)

	if side == "buy" {
		if execution.CumQty != nil && execution.CumQty.Sign() > 0 {
			position.Qty = execution.CumQty
		}

		if execution.CumQty == nil || execution.CumQty.Sign() <= 0 {
			if position.Qty == nil {
				position.Qty = execution.LastQty
			}

			if position.Qty != nil && position.Qty != execution.LastQty {
				position.Qty = position.Qty.Add(execution.LastQty)
			}
		}

		if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
			position.EntryPrice = execution.AvgPrice
		} else if execution.LastPrice != nil && execution.LastPrice.Sign() > 0 {
			position.EntryPrice = execution.LastPrice
		}

		if execution.FeeUsdEquiv != nil {
			if position.EntryFee == nil {
				position.EntryFee = execution.FeeUsdEquiv
			}

			if position.EntryFee != nil && position.EntryFee != execution.FeeUsdEquiv {
				position.EntryFee = position.EntryFee.Add(execution.FeeUsdEquiv)
			}
		}

		return
	}

	if side == "sell" {
		if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
			position.ExitPrice = execution.AvgPrice
		}

		if (execution.AvgPrice == nil || execution.AvgPrice.Sign() <= 0) && execution.LastPrice != nil {
			position.ExitPrice = execution.LastPrice
		}

		if execution.FeeUsdEquiv != nil {
			if position.ExitFee == nil {
				position.ExitFee = execution.FeeUsdEquiv
			}

			if position.ExitFee != nil && position.ExitFee != execution.FeeUsdEquiv {
				position.ExitFee = position.ExitFee.Add(execution.FeeUsdEquiv)
			}
		}

		if position.Qty != nil {
			position.Qty = position.Qty.Sub(execution.LastQty)

			if position.Qty.Sign() <= 0 {
				position.Qty = decimal.NewFromFloat64(0)
			}
		}
	}

	if position.ClosedAt != nil && position.ClosedAt.IsZero() {
		now := time.Now().UTC()
		position.ClosedAt = &now
	}
}
