package broker

import (
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
	mu          sync.RWMutex
	price       *Price
	ID          string                  `json:"id"`
	EntryPath   string                  `json:"entry_path"`
	ExitPath    string                  `json:"exit_path"`
	Symbol      string                  `json:"symbol"`
	OpenedAt    time.Time               `json:"opened_at"`
	ClosedAt    *time.Time              `json:"closed_at"`
	PolicyEpoch uint64                  `json:"policy_epoch,omitempty"`
	EntryTick   uint64                  `json:"entry_tick,omitempty"`
	ExitTick    uint64                  `json:"exit_tick,omitempty"`
	EntryOrders []*spot.OrderRequest    `json:"entry_orders,omitempty"`
	ExitOrders  []*spot.OrderRequest    `json:"exit_orders,omitempty"`
	Fills       []*kraken.ExecutionData `json:"fills,omitempty"`
	Qty         *decimal.Decimal        `json:"qty,omitempty"`
	EntryPrice  *decimal.Decimal        `json:"entry_price,omitempty"`
	ExitPrice   *decimal.Decimal        `json:"exit_price,omitempty"` // venue fill or close snapshot; not the live book walk
	EntryFee    *decimal.Decimal        `json:"entry_fee,omitempty"`
	ExitFee     *decimal.Decimal        `json:"exit_fee,omitempty"`
	Mark        *decimal.Decimal        `json:"mark,omitempty"`      // headline bid (or ask fallback)
	ExitVWAP    *decimal.Decimal        `json:"exit_vwap,omitempty"` // full-size sell through bids while open
	ExitRoom    *decimal.Decimal        `json:"exit_room,omitempty"`
	PnL         *decimal.Decimal        `json:"pnl,omitempty"`
	PnLPct      *decimal.Decimal        `json:"pnl_pct,omitempty"`
	OpenPnL     *decimal.Decimal        `json:"open_pnl,omitempty"` // first mark after virtual enter
	HoldNs      int64                   `json:"hold_ns,omitempty"`
	Virtual     bool                    `json:"virtual"`
	// PathFound is set when enter matched a stored explore or policy prefix,
	// not on the first virtual step that mints a new explore node.
	PathFound       bool     `json:"path_found,omitempty"`
	OutcomeTriggers []string `json:"outcome_triggers,omitempty"`
	EntryTradeRate  float64  `json:"entry_trade_rate,omitempty"`
	HoldTicks       int      `json:"hold_ticks,omitempty"`
	peakOpenPnLPct  float64  `json:"peak_open_pnl_pct,omitempty"`
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
virtual lot emulates the venue fill. After a successful Enter the position
fields reflect the entry.
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
	}

	return order, nil
}

/*
Exit records the sell and reprices from the bid book. After Exit the
position carries exit economics (venue fills may still arrive via Apply).
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
	position.PnL = position.price.PnL(symbol, qty, entry, fee)
	position.PnLPct = position.price.PnLPct(symbol, qty, entry, fee)

	if position.Virtual && position.ClosedAt == nil && position.OpenPnL == nil &&
		position.PnL != nil {
		position.OpenPnL = position.PnL
	}

	if position.ClosedAt == nil && position.PnLPct != nil {
		pct := position.PnLPct.Float64()
		if pct > position.peakOpenPnLPct {
			position.peakOpenPnLPct = pct
		}
	}
}

/*
ShouldExit is true when a virtual lot should close on book economics:
trailing giveback after profit, breakeven protection, or down by enough
to exceed initial friction with buffer against bid-ask bounce.
*/
func (position *Position) SetEntryTradeRate(rate float64) {
	position.mu.Lock()
	defer position.mu.Unlock()

	if rate > 0 {
		position.EntryTradeRate = rate
	}
}

func (position *Position) TickHold() {
	position.mu.Lock()
	defer position.mu.Unlock()

	if position.ClosedAt != nil {
		return
	}

	position.HoldTicks++

	if position.PnLPct == nil {
		return
	}

	pct := position.PnLPct.Float64()

	if pct > position.peakOpenPnLPct {
		position.peakOpenPnLPct = pct
	}
}

func (position *Position) GivebackFromPeak() float64 {
	position.mu.RLock()
	defer position.mu.RUnlock()

	if position.PnLPct == nil || position.peakOpenPnLPct <= 0 {
		return 0
	}

	pct := position.PnLPct.Float64()
	if pct >= position.peakOpenPnLPct {
		return 0
	}

	return (position.peakOpenPnLPct - pct) / position.peakOpenPnLPct
}

func (position *Position) ShouldExit() (trigger string, ok bool) {
	position.mu.RLock()
	defer position.mu.RUnlock()

	if position.PnL == nil {
		return "", false
	}

	// When in profit, let the trade run; only exit on trailing giveback or slip back to breakeven.
	if position.peakOpenPnLPct > 0 {
		pct := 0.0
		if position.PnLPct != nil {
			pct = position.PnLPct.Float64()
		}

		if pct <= 0.0 {
			return "breakeven protect", true
		}

		if position.peakOpenPnLPct >= 0.5 {
			giveback := (position.peakOpenPnLPct - pct) / position.peakOpenPnLPct
			if giveback >= 0.30 {
				return "trailing profit", true
			}
		}
	}

	// Downside stop: give 3x initial friction room so normal bid-ask spread
	// noise does not immediately stop out the position right after entry.
	if position.OpenPnL != nil && position.OpenPnL.Sign() < 0 {
		stop := position.OpenPnL.Mul(decimal.NewFromInt64(3))

		if position.PnL.Cmp(stop) <= 0 {
			return "friction dip", true
		}
	}

	return "", false
}

func (position *Position) SetOutcomeTriggers(triggers []string) {
	position.mu.Lock()
	defer position.mu.Unlock()

	position.OutcomeTriggers = triggers
}

func (position *Position) AppendOutcomeTrigger(trigger string) {
	if position == nil || trigger == "" {
		return
	}

	position.mu.Lock()
	position.OutcomeTriggers = append(position.OutcomeTriggers, trigger)
	position.mu.Unlock()
}
