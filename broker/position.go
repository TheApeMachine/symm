package broker

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
)

type Position struct {
	mu           sync.RWMutex
	transport    Transport
	price        *Price
	onSettled    func(*Position)
	ID           string                  `json:"id"`
	EntryPath    string                  `json:"entry_path"`
	ExitPath     string                  `json:"exit_path"`
	Symbol       string                  `json:"symbol"`
	OpenedAt     time.Time               `json:"opened_at"`
	ClosedAt     *time.Time              `json:"closed_at"`
	PolicyEpoch  uint64                  `json:"policy_epoch,omitempty"`
	EntryTick    uint64                  `json:"entry_tick,omitempty"`
	ExitTick     uint64                  `json:"exit_tick,omitempty"`
	EntryOrder   *spot.OrderRequest      `json:"entry_order,omitempty"`
	ExitOrder    *spot.OrderRequest      `json:"exit_order,omitempty"`
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
	OpenPnL      *decimal.Decimal        `json:"open_pnl,omitempty"` // First mark-to-exit PnL after open
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
	transport Transport,
	onSettled func(*Position),
) *Position {
	return &Position{
		price:       price,
		transport:   transport,
		onSettled:   onSettled,
		ID:          uuid.NewString(),
		Fills: make([]*kraken.ExecutionData, 0),
		Symbol:      symbol,
		EntryPath:   path,
		Virtual:     virtual,
	}
}

/*
Update refreshes the mark, reprices open PnL, and exits immediately when
ShouldExit fires — no wait for the next workspace tick.
*/
func (position *Position) Update(mark *decimal.Decimal) {
	var (
		path   string
		closed bool
	)

	position.mu.Lock()

	if position.ClosedAt == nil {
		position.Mark = mark
		position.repriceLocked()

		if _, ok := position.shouldExitLocked(); ok {
			path = position.ExitPath
			closed = path != ""
		}
	}

	position.mu.Unlock()

	if !closed {
		return
	}

	if err := position.Exit(path); err != nil {
		errnie.Error(err)
		return
	}

	if position.onSettled != nil {
		position.onSettled(position)
	}
}

/*
Enter sizes from the book when qty is unset, records the buy, and for a
virtual lot emulates the venue fill.
*/
func (position *Position) Enter(path string) error {
	if path == "" {
		return errnie.Err(
			errnie.NotFound,
			"[position.Enter] missing path for "+position.Symbol,
			nil,
		)
	}

	qty := position.Qty
	if qty == nil {
		var why string
		qty, why = position.price.Allocate(position.Symbol)
		if qty == nil || qty.Sign() <= 0 {
			msg := "[position.Enter] cannot size " + position.Symbol
			if why != "" {
				msg += ": " + why
			}

			return errnie.Err(errnie.NotFound, msg, nil)
		}
	}

	if qty.Sign() <= 0 {
		return errnie.Err(
			errnie.NotFound,
			"[position.Enter] non-positive quantity for "+position.Symbol,
			nil,
		)
	}

	vol := qty.String()
	entryOrder := &spot.OrderRequest{
		ClOrdId:   uuid.NewString(),
		OrderType: "market",
		Type:      "buy",
		Volume:    vol,
	}
	exitOrder := &spot.OrderRequest{
		ClOrdId:   uuid.NewString(),
		OrderType: "market",
		Type:      "sell",
		Volume:    vol,
	}

	now := time.Now().UTC()
	position.mu.Lock()
	position.Qty = qty
	position.EntryOrder = entryOrder
	position.ExitOrder = exitOrder
	position.OpenedAt = now
	position.PolicyEpoch = data.Epoch.Load()
	position.EntryTick = data.Tick.Load()
	position.EntryPath = path
	position.ExitPath = path
	virtual := position.Virtual
	position.mu.Unlock()

	if virtual {
		fill, err := position.price.EntryCost(position.Symbol, qty)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				"[position.Enter] no fill price for "+position.Symbol,
				err,
			))
		}

		position.Apply(kraken.ExecutionData{
			Symbol:      position.Symbol,
			Side:        "buy",
			ExecType:    "Fill",
			LastQty:     qty,
			CumQty:      qty,
			LastPrice:   fill.EntryPrice,
			AvgPrice:    fill.EntryPrice,
			FeeUsdEquiv: fill.EntryFee,
			Timestamp:   time.Now().UTC(),
		})

		if err := position.rejectOpenBookDrag(); err != nil {
			return err
		}
	}

	return position.Send(entryOrder)
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
func (position *Position) Exit(path string) error {
	if path == "" {
		return errnie.Err(
			errnie.NotFound,
			"[position.Exit] missing path for "+position.Symbol,
			nil,
		)
	}

	position.mu.Lock()
	defer position.mu.Unlock()

	if position.ExitOrder == nil {
		return errnie.Err(
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

	return position.Send(position.ExitOrder)
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

	if position.OpenPnL == nil && position.ClosedAt == nil {
		position.OpenPnL = newPnL
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
	return position.shouldExitLocked()
}

func (position *Position) shouldExitLocked() (trigger string, ok bool) {
	if position.PnL == nil || position.ClosedAt != nil {
		return "", false
	}

	// 1. DOWNSIDE STOP:
	if position.OpenPnL != nil && position.OpenPnL.Sign() < 0 {
		stop := position.OpenPnL.Mul(decimal.NewFromInt64(2))
		if position.PnL.Cmp(stop) <= 0 {
			return "symmetric stop", true
		}
	}

	// 2. PROFIT GIVEBACK:
	if position.LastPnL != nil &&
		position.LastPnL.Sign() > 0 &&
		position.PnL.Sign() <= 0 {
		return "profit giveback", true
	}

	// 3. UPSIDE MOMENTUM EXIT:
	if position.LastPnL != nil &&
		position.PnL.Sign() > 0 &&
		position.PnL.Cmp(position.LastPnL) <= 0 {
		return "momentum exit", true
	}

	return "", false
}

/*
send an order to the exchange.
*/
func (position *Position) Send(order *spot.OrderRequest) error {
	if position.Virtual {
		return nil
	}

	if order == nil || position.Symbol == "" {
		return errnie.Err(
			errnie.NotFound,
			"[desk.Send] paper order is missing its symbol",
			nil,
		)
	}

	buf, err := sonic.Marshal(kraken.NewAddOrderMessage("", &kraken.AddOrderRequest{
		Pair:    position.Symbol,
		Type:    order.Type,
		OrdType: order.OrderType,
		Volume:  order.Volume,
		Price:   order.Price,
		ClOrdId: order.ClOrdId,
	}))

	if err != nil {
		return errnie.Err(
			errnie.IO,
			"[desk.Send] failed to marshal order",
			err,
		)
	}

	return position.transport.Write(buf)
}
