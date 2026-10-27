package broker

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
)

type Position struct {
	mu           sync.RWMutex
	price        *Price
	ID           string                  `json:"id"`
	EntryPath    string                  `json:"entry_path"`
	ExitPath     string                  `json:"exit_path"`
	Symbol       string                  `json:"symbol"`
	OpenedAt     time.Time               `json:"opened_at"`
	ClosedAt     *time.Time              `json:"closed_at"`
	PolicyEpoch  uint64                  `json:"policy_epoch,omitempty"`
	EntryTick    uint64                  `json:"entry_tick,omitempty"`
	ExitTick     uint64                  `json:"exit_tick,omitempty"`
	EntryOrders  []*spot.OrderRequest    `json:"entry_orders,omitempty"`
	ExitOrders   []*spot.OrderRequest    `json:"exit_orders,omitempty"`
	Fills        []*kraken.ExecutionData `json:"fills,omitempty"`
	Qty          *decimal.Decimal        `json:"qty,omitempty"`
	EntryPrice   *decimal.Decimal        `json:"entry_price,omitempty"`
	ExitPrice    *decimal.Decimal        `json:"exit_price,omitempty"`
	EntryFee     *decimal.Decimal        `json:"entry_fee,omitempty"`
	ExitFee      *decimal.Decimal        `json:"exit_fee,omitempty"`
	Mark         *decimal.Decimal        `json:"mark,omitempty"`
	ExitVWAP     *decimal.Decimal        `json:"exit_vwap,omitempty"`
	ExitRoom     *decimal.Decimal        `json:"exit_room,omitempty"`
	PnL          *decimal.Decimal        `json:"pnl,omitempty"`
	PnLPct       *decimal.Decimal        `json:"pnl_pct,omitempty"`
	LastPnL      *decimal.Decimal        `json:"last_pnl,omitempty"` // Previous tick PnL for momentum checks
	HoldNs       int64                   `json:"hold_ns,omitempty"`
	Virtual      bool                    `json:"virtual"`
	PathFound    bool                    `json:"path_found,omitempty"`
	OpenSurprise float64                 `json:"open_surprise,omitempty"`
}

func NewPosition(
	symbol string,
	path string,
	virtual bool,
	price *Price,
) *Position {
	return &Position{
		price:       price,
		ID:          uuid.NewString(),
		EntryOrders: make([]*spot.OrderRequest, 0),
		ExitOrders:  make([]*spot.OrderRequest, 0),
		Fills:       make([]*kraken.ExecutionData, 0),
		Symbol:      symbol,
		EntryPath:   path,
		Virtual:     virtual,
	}
}

/*
Update refreshes the mark and reprices open PnL from the book.
*/
func (position *Position) Update(mark *decimal.Decimal) {
	position.mu.Lock()
	defer position.mu.Unlock()

	position.Mark = mark
	position.repriceLocked()
}

/*
Enter sizes from the book when qty is unset, records the buy, and for a
virtual lot emulates the venue fill.
*/
func (position *Position) Enter(path string) (*spot.OrderRequest, error) {
	if path == "" {
		return nil, errnie.Err(
			errnie.NotFound,
			"[position.Enter] missing path for "+position.Symbol,
			nil,
		)
	}

	position.mu.Lock()

	if position.Qty == nil {
		var why string
		position.Qty, why = position.price.Allocate(position.Symbol)
		if position.Qty == nil || position.Qty.Sign() <= 0 {
			position.mu.Unlock()

			msg := "[position.Enter] cannot size " + position.Symbol
			if why != "" {
				msg += ": " + why
			}

			return nil, errnie.Err(errnie.NotFound, msg, nil)
		}
	}

	if position.Qty.Sign() <= 0 {
		position.mu.Unlock()

		return nil, errnie.Err(
			errnie.NotFound,
			"[position.Enter] non-positive quantity for "+position.Symbol,
			nil,
		)
	}

	position.EntryOrders = append(position.EntryOrders, &spot.OrderRequest{
		ClOrdId:   uuid.NewString(),
		OrderType: "market",
		Type:      "buy",
		Volume:    position.Qty.String(),
	})

	position.ExitOrders = append(position.ExitOrders, &spot.OrderRequest{
		ClOrdId:   uuid.NewString(),
		OrderType: "market",
		Type:      "sell",
		Volume:    position.Qty.String(),
	})

	position.OpenedAt = time.Now().UTC()
	position.PolicyEpoch = data.Epoch.Load()
	position.EntryTick = data.Tick.Load()
	position.EntryPath = path
	virtual := position.Virtual

	order := position.EntryOrders[len(position.EntryOrders)-1]
	position.mu.Unlock()

	if virtual {
		fill, err := position.price.EntryCost(position.Symbol, position.Qty)

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"[position.Enter] no fill price for "+position.Symbol,
				err,
			))
		}

		position.Apply(kraken.ExecutionData{
			Symbol:      position.Symbol,
			Side:        "buy",
			ExecType:    "Fill",
			LastQty:     position.Qty,
			CumQty:      position.Qty,
			LastPrice:   fill.EntryPrice,
			AvgPrice:    fill.EntryPrice,
			FeeUsdEquiv: fill.EntryFee,
			Timestamp:   time.Now().UTC(),
		})

		if err := position.rejectOpenBookDrag(); err != nil {
			return nil, err
		}
	}

	return order, nil
}

func (position *Position) rejectOpenBookDrag() error {
	if position == nil || position.price == nil {
		return nil
	}

	position.mu.RLock()
	pnlPct := position.PnLPct
	position.mu.RUnlock()

	if pnlPct == nil || pnlPct.Sign() >= 0 {
		return nil
	}

	budget, ok := position.price.OpenDragBudgetPct(position.Symbol)
	if !ok || budget == nil {
		return nil
	}

	maxLoss := budget.Mul(decimal.NewFromInt64(-1))
	if pnlPct.Cmp(maxLoss) >= 0 {
		return nil
	}

	return errnie.Err(
		errnie.UnprocessableContent,
		fmt.Sprintf(
			"[position.Enter] open book drag %s%% exceeds friction budget %s%% for %s",
			pnlPct.String(),
			maxLoss.String(),
			position.Symbol,
		),
		nil,
	)
}

/*
Exit records the sell and reprices from the bid book.
*/
func (position *Position) Exit(path string) (*spot.OrderRequest, error) {
	if path == "" {
		return nil, errnie.Err(
			errnie.NotFound,
			"[position.Exit] missing path for "+position.Symbol,
			nil,
		)
	}

	position.mu.Lock()
	defer position.mu.Unlock()

	if len(position.ExitOrders) == 0 {
		return nil, errnie.Err(
			errnie.NotFound, "[position.Exit] no exit order for "+position.Symbol, nil,
		)
	}

	now := time.Now().UTC()
	position.ClosedAt = &now
	position.ExitPath = path
	position.ExitVWAP = position.price.ExitVWAP(position.Symbol, position.Qty)
	position.ExitPrice = position.ExitVWAP

	if !position.OpenedAt.IsZero() {
		position.HoldNs = int64(now.Sub(position.OpenedAt))
	}

	position.ExitTick = data.Tick.Load()
	position.repriceLocked()

	return position.ExitOrders[len(position.ExitOrders)-1], nil
}

func (position *Position) Apply(execution kraken.ExecutionData) {
	position.mu.Lock()
	defer position.mu.Unlock()

	position.Fills = append(position.Fills, &execution)

	if execution.LastQty == nil || execution.LastQty.Sign() <= 0 {
		return
	}

	side := strings.ToLower(execution.Side)

	if side == "buy" {
		if execution.CumQty != nil && execution.CumQty.Sign() > 0 {
			position.Qty = execution.CumQty
		} else if position.Qty == nil {
			position.Qty = execution.LastQty
		} else if position.Qty != execution.LastQty {
			position.Qty = position.Qty.Add(execution.LastQty)
		}

		if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
			position.EntryPrice = execution.AvgPrice
		} else if execution.LastPrice != nil && execution.LastPrice.Sign() > 0 {
			position.EntryPrice = execution.LastPrice
		}

		if position.Mark == nil {
			position.Mark = position.EntryPrice
		}

		position.repriceLocked()
		return
	}

	if side == "sell" {
		if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
			position.ExitPrice = execution.AvgPrice
		} else if execution.LastPrice != nil && execution.LastPrice.Sign() > 0 {
			position.ExitPrice = execution.LastPrice
		}

		if position.Qty != nil {
			position.Qty = position.Qty.Sub(execution.LastQty)

			if position.Qty.Sign() <= 0 {
				position.Qty = decimal.NewFromFloat64(0)
			}
		}

		position.repriceLocked()
	}

	if position.ClosedAt != nil && position.ClosedAt.IsZero() {
		now := time.Now().UTC()
		position.ClosedAt = &now
	}
}

func (position *Position) repriceLocked() {
	if position.price == nil {
		return
	}

	qty := position.Qty
	if qty == nil || qty.Sign() <= 0 {
		return
	}

	fee := position.price.Fee(position.Symbol)
	symbol := position.Symbol
	entry := position.EntryPrice

	position.ExitVWAP = position.price.ExitVWAP(symbol, qty)
	position.ExitRoom = position.price.ExitRoom(symbol, qty)

	newPnL := position.price.PnL(symbol, qty, entry, fee)
	newPnLPct := position.price.PnLPct(symbol, qty, entry, fee)

	if newPnL == nil {
		return
	}

	// Save the previous tick's PnL to detect momentum stalls
	if position.PnL != nil {
		position.LastPnL = position.PnL
	}

	position.PnL = newPnL
	position.PnLPct = newPnLPct

	// Establish entry baseline PnL (initial fee & spread drag)
	if position.ClosedAt == nil && position.LastPnL == nil {
		position.LastPnL = position.PnL
	}
}

/*
ShouldExit checks two conditions:
1. Downside: PnL dropped as much as it needed to gain to reach breakeven (2 * OpenPnL).
2. Upside: Upwards momentum exhausted, flatlined, or reversed once in profit.
*/
func (position *Position) ShouldExit() (trigger string, ok bool) {
	position.mu.RLock()
	defer position.mu.RUnlock()

	if position.PnL == nil {
		return "", false
	}

	// 1. DOWNSIDE STOP:
	// OpenPnL is negative (e.g. -$10). It needed +$10 to reach breakeven ($0).
	// If it drops that same $10 downwards, it hits -$20 (2 * OpenPnL).
	if position.PnL != nil && position.PnL.Sign() < 0 {
		stopLoss := position.PnL.Mul(decimal.NewFromInt64(2))

		if position.PnL.Cmp(stopLoss) <= 0 {
			return "symmetric stop", true
		}
	}

	// 2. UPSIDE MOMENTUM EXIT:
	// Only triggers once the position has shown upward momentum into profit.
	if position.PnL.Sign() > 0 && position.PnL.Cmp(position.LastPnL) <= 0 {
		return "momentum exit", true
	}

	return "", false
}
