package broker

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/network"
)

type Position struct {
	*runtime.System
	mu              sync.Mutex
	transport       network.Client
	price           *Price
	onSettled       func(*Position)
	Stoploss        *Stoploss               `json:"stoploss,omitempty"`
	ID              string                  `json:"id"`
	TierKey         string                  `json:"tier_key,omitempty"`
	EntryPath       string                  `json:"entry_path"`
	HoldPaths       []string                `json:"hold_paths,omitempty"`
	ExitPath        string                  `json:"exit_path"`
	ExitTrigger     string                  `json:"exit_trigger,omitempty"`
	OutcomeTriggers []string                `json:"outcome_triggers,omitempty"`
	Symbol          string                  `json:"symbol"`
	OpenedAt        time.Time               `json:"opened_at"`
	ClosedAt        *time.Time              `json:"closed_at"`
	UpdatedAt       *time.Time              `json:"updated_at"`
	PolicyEpoch     uint64                  `json:"policy_epoch,omitempty"`
	EntryTick       uint64                  `json:"entry_tick,omitempty"`
	ExitTick        uint64                  `json:"exit_tick,omitempty"`
	EntryOrder      *spot.OrderRequest      `json:"entry_order,omitempty"`
	ExitOrder       *spot.OrderRequest      `json:"exit_order,omitempty"`
	Fills           []*market.ExecutionData `json:"fills,omitempty"`
	Qty             *decimal.Decimal        `json:"qty,omitempty"`
	Entry           *decimal.Decimal        `json:"entry,omitempty"`
	Sold            *decimal.Decimal        `json:"sold,omitempty"`
	Mark            *decimal.Decimal        `json:"mark,omitempty"`
	ExitVWAP        *decimal.Decimal        `json:"exit_vwap,omitempty"`
	ExitRoom        *decimal.Decimal        `json:"exit_room,omitempty"`
	PnL             *decimal.Decimal        `json:"pnl,omitempty"`
	PnLPct          *decimal.Decimal        `json:"pnl_pct,omitempty"`
	Closing         bool                    `json:"closing,omitempty"`
	// TreeApplied is set after Model.Exit trains resistance for this lot.
	TreeApplied bool `json:"-"`
}

func NewPosition(
	ctx context.Context,
	symbol string,
	tierKey string,
	path string,
	price *Price,
	transport network.Client,
	onSettled func(*Position),
) *Position {
	return &Position{
		System:    runtime.NewSystem(ctx, "position"),
		price:     price,
		transport: transport,
		onSettled: onSettled,
		ID:        uuid.NewString(),
		TierKey:   tierKey,
		Fills:     make([]*market.ExecutionData, 0),
		Symbol:    symbol,
		EntryPath: path,
	}
}

/*
Update marks an open lot, feeds the stoploss, and on trigger arms Closing.
ClosedAt waits for exit fills in Apply. Calling Update on a lot that is not
a live open fill is a bug — it errors, it does not soft-skip.
*/
func (position *Position) Update(price *decimal.Decimal) error {
	position.mu.Lock()

	if price == nil {
		position.mu.Unlock()
		return errnie.Error(errnie.Err(
			errnie.Conflict,
			"[position.Update] price is nil",
			nil,
		), "symbol", position.Symbol)
	}

	if position.ClosedAt != nil {
		position.mu.Unlock()
		return errnie.Error(errnie.Err(
			errnie.Conflict,
			"[position.Update] closed lot",
			nil,
		), "symbol", position.Symbol)
	}

	if position.price == nil {
		position.mu.Unlock()
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[position.Update] missi	ng price",
			nil,
		), "symbol", position.Symbol)
	}

	if position.Qty == nil || position.Qty.Sign() <= 0 ||
		position.Entry == nil || position.Entry.Sign() <= 0 {
		position.mu.Unlock()
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[position.Update] lot not filled",
			nil,
		), "symbol", position.Symbol)
	}

	position.Mark = price

	fee := position.price.Fee(position.Symbol)
	if fee == nil {
		zero := decimal.Zero
		fee = &zero
	}

	now := time.Now().UTC()
	position.UpdatedAt = &now

	// Snapshot under the position lock, then walk the book unlocked so marks
	// do not hold Position.mu across BookManager.RLock.
	symbol := position.Symbol
	qty := *position.Qty
	entry := *position.Entry
	feeVal := *fee
	pricer := position.price
	position.mu.Unlock()

	vwap, room, bookPnL, bookPnLPct := pricer.closeNow(
		symbol, &qty, &entry, &feeVal,
	)

	position.mu.Lock()

	if position.ClosedAt != nil || position.Qty == nil || position.Entry == nil {
		position.mu.Unlock()
		return nil
	}

	if vwap != nil {
		position.ExitVWAP = vwap
	}

	position.ExitRoom = room

	if bookPnL != nil {
		position.PnL = bookPnL
		position.PnLPct = bookPnLPct
	} else {
		position.feeNetPnL(position.Entry, position.Qty, price, &feeVal)
	}

	if position.PnLPct == nil {
		position.mu.Unlock()
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[position.Update] no pnl for "+position.Symbol,
			nil,
		))
	}

	if position.Closing {
		position.mu.Unlock()
		return nil
	}

	pnlPct, _ := position.PnLPct.Float64()

	if position.Stoploss == nil {
		position.Stoploss = NewStoploss(pnlPct)
	}

	position.Stoploss.Update(pnlPct)

	if !position.Stoploss.Triggered() {
		position.mu.Unlock()
		return nil
	}

	position.armClosing("stoploss")
	onSettled := position.onSettled
	position.mu.Unlock()

	if onSettled != nil {
		onSettled(position)
	}

	return nil
}

/*
Enter records the buy order. qty must be a venue-valid size from Price.Allocate;
when nil or non-positive, Allocate sizes once from wallet. Fills arrive only
through Apply.
*/
func (position *Position) Enter(path string, qty *decimal.Decimal, wallet *market.Balance) error {
	position.mu.Lock()
	defer position.mu.Unlock()

	if path == "" {
		return errnie.Err(
			errnie.NotFound,
			"[position.Enter] missing path for "+position.Symbol,
			nil,
		)
	}

	if qty == nil || qty.Sign() <= 0 {
		var err error
		qty, err = position.price.Allocate(position.Symbol, wallet)

		if err != nil || qty == nil || qty.Sign() <= 0 {
			if err != nil {
				return errnie.Error(err)
			}

			return errnie.Err(
				errnie.UnprocessableContent,
				"[position.Enter] cannot size "+position.Symbol,
				nil,
			)
		}
	}

	vol := qty.String()

	position.EntryOrder = &spot.OrderRequest{
		ClOrdId:   uuid.NewString(),
		OrderType: "market",
		Type:      "buy",
		Volume:    vol,
	}

	position.ExitOrder = &spot.OrderRequest{
		ClOrdId:   uuid.NewString(),
		OrderType: "market",
		Type:      "sell",
		Volume:    vol,
	}

	now := time.Now().UTC()
	position.OpenedAt = now
	position.PolicyEpoch = data.Epoch.Load()
	position.EntryTick = data.Tick.Load()
	position.EntryPath = path

	return nil
}

/*
Restock credits a virtual wallet for a lot reopened from S3 (fresh process has
quote cash only). Uses open Qty/Entry only.
*/
func (position *Position) Restock(virtual *network.Virtual) {
	position.mu.Lock()
	defer position.mu.Unlock()

	qty := position.Qty
	entry := position.Entry

	if qty == nil || qty.Sign() <= 0 {
		return
	}

	cost := decimal.Zero

	if entry != nil && entry.Sign() > 0 {
		gross := qty.Mul(*entry)
		cost = gross

		if position.price != nil {
			fee := position.price.Fee(position.Symbol)
			if fee == nil {
				zero := decimal.Zero
				fee = &zero
			}

			if net := position.price.afterFee(&gross, fee, BUY); net != nil {
				cost = *net
			}
		}
	}

	virtual.Restock(position.Symbol, *qty, cost)
}

/*
Owns reports whether this execution belongs to the lot's entry or exit order.
*/
func (position *Position) Owns(execution market.ExecutionData) bool {
	if execution.ClientOrderID == "" {
		return false
	}

	if position.EntryOrder != nil &&
		position.EntryOrder.ClOrdId == execution.ClientOrderID {
		return true
	}

	if position.ExitOrder != nil &&
		position.ExitOrder.ClOrdId == execution.ClientOrderID {
		return true
	}

	return false
}

// feeNetPnL sets fee-net PnL from an explicit exit price.
func (position *Position) feeNetPnL(
	entry, qty, exit, fee *decimal.Decimal,
) {
	if entry == nil || entry.Sign() <= 0 ||
		qty == nil || qty.Sign() <= 0 ||
		exit == nil || exit.Sign() <= 0 ||
		position.price == nil {
		return
	}

	if fee == nil {
		zero := decimal.Zero
		fee = &zero
	}

	entryCost := entry.Mul(*qty)
	netEntry := position.price.afterFee(&entryCost, fee, BUY)
	exitGross := exit.Mul(*qty)
	netExit := position.price.afterFee(&exitGross, fee, SELL)

	if netEntry == nil || netExit == nil || netEntry.Sign() <= 0 {
		return
	}

	pnl := netExit.Sub(*netEntry)
	position.PnL = &pnl
	pct := pnl.Div(*netEntry).Mul(decimalHundred)
	position.PnLPct = &pct
}

/*
CloseNowProceeds is fee-net quote received if the open lot sold now.
*/
func (position *Position) CloseNowProceeds() *decimal.Decimal {
	position.mu.Lock()
	defer position.mu.Unlock()

	if position.price == nil || position.Qty == nil || position.Qty.Sign() <= 0 {
		return nil
	}

	fee := position.price.Fee(position.Symbol)

	if fee == nil {
		zero := decimal.Zero
		fee = &zero
	}

	if position.PnL != nil && position.Entry != nil && position.Entry.Sign() > 0 {
		entryCost := position.Entry.Mul(*position.Qty)
		netEntry := position.price.afterFee(&entryCost, fee, BUY)

		if netEntry != nil {
			proceeds := position.PnL.Add(*netEntry)
			return &proceeds
		}
	}

	exit := position.ExitVWAP

	if exit == nil || exit.Sign() <= 0 {
		exit = position.Mark
	}

	if exit == nil || exit.Sign() <= 0 {
		return nil
	}

	gross := exit.Mul(*position.Qty)
	return position.price.afterFee(&gross, fee, SELL)
}

// Flat is true when the lot has sold its open size.
func (position *Position) Flat() bool {
	return position.Sold != nil && position.Sold.Sign() > 0 &&
		(position.Qty == nil || position.Qty.Sign() <= 0)
}

/*
Exit arms Closing for a policy exit. ClosedAt waits for exit fills in Apply.
*/
func (position *Position) Exit() error {
	position.mu.Lock()

	if position.ClosedAt != nil || position.Closing {
		position.mu.Unlock()
		return nil
	}

	trigger := "policy exit match " + position.ExitPath

	if position.ExitTrigger == "" {
		position.armClosing(trigger)
	} else {
		position.armClosing(position.ExitTrigger)
	}

	position.mu.Unlock()
	return nil
}

/*
Apply is the only fill path. It updates Qty/Entry/Sold from the print, and
only stamps ClosedAt once Closing is armed and the lot is flat.
*/
func (position *Position) Apply(execution market.ExecutionData) {
	position.mu.Lock()

	if execution.LastQty == nil || execution.LastQty.Sign() <= 0 {
		position.mu.Unlock()
		return
	}

	if execution.ExecID != "" {
		for _, fill := range position.Fills {
			if fill != nil && fill.ExecID == execution.ExecID {
				position.mu.Unlock()
				return
			}
		}
	}

	side := strings.ToLower(execution.Side)
	px := execution.LastPrice

	if px == nil || px.Sign() <= 0 {
		px = execution.AvgPrice
	}

	if px == nil || px.Sign() <= 0 {
		position.mu.Unlock()
		return
	}

	qty := *execution.LastQty

	switch side {
	case "buy":
		position.applyBuy(qty, *px)
	case "sell":
		if position.Qty == nil || position.Qty.Sign() <= 0 {
			position.mu.Unlock()
			return
		}
		position.applySell(qty, *px)
	default:
		position.mu.Unlock()
		return
	}

	copied := execution
	position.Fills = append(position.Fills, &copied)
	position.Mark = px

	now := time.Now().UTC()
	position.UpdatedAt = &now

	closed := false

	if position.Flat() {
		fee := decimal.Zero

		if position.price != nil {
			if rate := position.price.Fee(position.Symbol); rate != nil {
				fee = *rate
			}
		}

		position.feeNetPnL(position.Entry, position.Sold, position.ExitVWAP, &fee)

		if position.Closing && position.ClosedAt == nil {
			position.ClosedAt = &now

			if position.ExitTick == 0 {
				position.ExitTick = data.Tick.Load()
			}

			closed = true
		}
	}

	onSettled := position.onSettled
	position.mu.Unlock()

	if closed && onSettled != nil {
		onSettled(position)
	}
}

func (position *Position) applyBuy(qty, px decimal.Decimal) {
	if position.Qty == nil || position.Qty.Sign() <= 0 ||
		position.Entry == nil || position.Entry.Sign() <= 0 {
		q := qty
		e := px
		position.Qty = &q
		position.Entry = &e
		return
	}

	cost := position.Entry.Mul(*position.Qty).Add(px.Mul(qty))
	open := position.Qty.Add(qty)
	entry := cost.Div(open)
	position.Qty = &open
	position.Entry = &entry
}

func (position *Position) applySell(qty, px decimal.Decimal) {
	take := qty

	if take.Cmp(*position.Qty) > 0 {
		take = *position.Qty
	}

	if take.Sign() <= 0 {
		return
	}

	prevSold := decimal.Zero
	if position.Sold != nil {
		prevSold = *position.Sold
	}

	sold := prevSold.Add(take)
	position.Sold = &sold

	if position.ExitVWAP != nil && position.ExitVWAP.Sign() > 0 && prevSold.Sign() > 0 {
		vwap := position.ExitVWAP.Mul(prevSold).Add(px.Mul(take)).Div(sold)
		position.ExitVWAP = &vwap
	} else {
		position.ExitVWAP = &px
	}

	open := position.Qty.Sub(take)

	if open.Sign() < 0 {
		open = decimal.Zero
	}

	position.Qty = &open
}

func (position *Position) armClosing(trigger string) {
	position.Closing = true
	position.ExitTrigger = trigger
	position.OutcomeTriggers = []string{trigger}
	position.ExitTick = data.Tick.Load()
}
