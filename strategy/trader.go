package strategy

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/runtime"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
Trader talks to the broker, owns open positions, and publishes the position /
decision / equity frames the hub streams to the dashboard.
*/
type Trader struct {
	*runtime.System
	desk             *broker.Desk
	price            *broker.Price
	balance          *broker.Balance
	mu               sync.Mutex
	open             map[string]*position.Regulator
	positionsVersion atomic.Uint64
	decisionsVersion atomic.Uint64
	decisions        atomic.Pointer[[]*wire.DecisionT]
}

func NewTrader(
	ctx context.Context,
	private broker.Transport,
	price *broker.Price,
	balance *broker.Balance,
) *Trader {
	trader := &Trader{
		System:  runtime.NewSystem(ctx, "trader"),
		desk:    broker.NewDesk(ctx, private, price, balance),
		price:   price,
		balance: balance,
		open:    make(map[string]*position.Regulator),
	}

	trader.positionsVersion.Store(1)
	trader.decisionsVersion.Store(1)
	initial := make([]*wire.DecisionT, 0, 50)
	trader.decisions.Store(&initial)

	// Paper synthesizes Execution frames in Write; without this handler
	// Regulator.Reconcile never sees fills and inventory stays phantom.
	if paper, ok := private.(*broker.Paper); ok && paper != nil {
		paper.OnExecution(trader.ApplyExecution)
	}

	return trader
}

func (trader *Trader) Position(symbol string) *position.Regulator {
	if trader == nil {
		return nil
	}

	trader.mu.Lock()
	defer trader.mu.Unlock()

	return trader.open[symbol]
}

func (trader *Trader) OnAction(label string, action cognition.Action, confidence float64) error {
	if trader == nil || trader.desk == nil || trader.desk.Execution == nil || label == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: execution is not available",
			nil,
		))
	}

	if action == cognition.ActionExit {
		return trader.exit(label, confidence)
	}

	if action == cognition.ActionEnter {
		return trader.enter(label, confidence)
	}

	return nil
}

func (trader *Trader) enter(symbol string, confidence float64) error {
	current := trader.Position(symbol)

	if current != nil && !current.IsClosed() {
		return nil
	}

	// Own the regulator under onPending BEFORE Write can synthesize a fill,
	// otherwise ApplyExecution cannot Identifies/Reconcile the race.
	regulator, err := trader.desk.Execution.Enter(symbol, func(reg *position.Regulator) {
		trader.mu.Lock()
		defer trader.mu.Unlock()

		if trader.open == nil {
			trader.open = make(map[string]*position.Regulator)
		}

		trader.open[symbol] = reg
	})

	if err != nil {
		// Begin never ran → IsClosed; keep ownership if a pending order exists
		// so a racing fill can still Reconcile.
		if regulator != nil && regulator.IsClosed() {
			trader.mu.Lock()
			if trader.open[symbol] == regulator {
				delete(trader.open, symbol)
			}
			trader.mu.Unlock()
		}

		trader.RecordDecision(symbol, "blocked", confidence, fmt.Sprintf("enter failed: %v", err))
		return err
	}

	trader.positionsVersion.Add(1)
	trader.RecordDecision(symbol, "enter", confidence, "enter")

	return nil
}

/*
ApplyExecution routes venue/paper execution reports onto the matching open
Regulator via Reconcile. No invent: only reports that Identifies an owned order.
*/
func (trader *Trader) ApplyExecution(execution *kraken.Execution) {
	if trader == nil || execution == nil {
		return
	}

	for index := range execution.Data {
		report := execution.Data[index]
		trader.applyReport(report)
	}
}

func (trader *Trader) applyReport(report kraken.ExecutionData) {
	trader.mu.Lock()
	regs := make([]*position.Regulator, 0, len(trader.open))

	for _, reg := range trader.open {
		regs = append(regs, reg)
	}

	trader.mu.Unlock()

	for _, reg := range regs {
		if reg == nil || !reg.Identifies(report.OrderID, report.ClientOrderID) {
			continue
		}

		if err := reg.Reconcile(report); err != nil {
			errnie.Error(err)
			continue
		}

		if reg.IsClosed() {
			trader.mu.Lock()
			if trader.open[reg.Symbol] == reg {
				delete(trader.open, reg.Symbol)
			}
			trader.mu.Unlock()
			trader.positionsVersion.Add(1)
		}

		return
	}
}

func (trader *Trader) exit(symbol string, confidence float64) error {
	current := trader.Position(symbol)

	if current == nil || !current.IsHolding() || current.Status() == "exit_pending" {
		return nil
	}

	if err := trader.desk.Execution.Exit(current); err != nil {
		trader.RecordDecision(symbol, "exit_failed", confidence, fmt.Sprintf("exit failed: %v", err))
		return err
	}

	if current.IsClosed() {
		trader.mu.Lock()
		delete(trader.open, symbol)
		trader.mu.Unlock()
		trader.positionsVersion.Add(1)
	}

	trader.RecordDecision(symbol, "exit", confidence, "exit")

	return nil
}

func (trader *Trader) RecordDecision(symbol string, action string, confidence float64, reason string) {
	if trader == nil || symbol == "" {
		return
	}

	decision := &wire.DecisionT{
		Id:         fmt.Sprintf("dec-%s-%d", symbol, time.Now().UnixNano()),
		Symbol:     symbol,
		Action:     action,
		Confidence: confidence,
		Reason:     reason,
		At:         time.Now().UnixNano(),
	}

	for {
		oldPtr := trader.decisions.Load()
		oldSlice := []*wire.DecisionT{}

		if oldPtr != nil {
			oldSlice = *oldPtr
		}

		start := 0

		if len(oldSlice) >= 50 {
			start = len(oldSlice) - 49
		}

		next := append(append([]*wire.DecisionT{}, oldSlice[start:]...), decision)

		if trader.decisions.CompareAndSwap(oldPtr, &next) {
			trader.decisionsVersion.Add(1)
			return
		}
	}
}

func (trader *Trader) PositionsVersion() uint64 {
	if trader == nil {
		return 0
	}

	return trader.positionsVersion.Load()
}

func (trader *Trader) DecisionsVersion() uint64 {
	if trader == nil {
		return 0
	}

	return trader.decisionsVersion.Load()
}

func (trader *Trader) DecisionsWire() *wire.StrategyFrameT {
	if trader == nil {
		return &wire.StrategyFrameT{Evaluated: true, Decisions: []*wire.DecisionT{}}
	}

	decPtr := trader.decisions.Load()
	decisions := []*wire.DecisionT{}

	if decPtr != nil {
		decisions = append(decisions, (*decPtr)...)
	}

	return &wire.StrategyFrameT{
		Evaluated: true,
		Outcome:   "active",
		Decisions: decisions,
	}
}

func (trader *Trader) EquityWire() *wire.EquityFrameT {
	empty := &wire.EquityFrameT{Cash: "0.00", Unrealized: "0.00", Equity: "0.00"}

	if trader == nil || trader.balance == nil {
		return empty
	}

	snap := trader.balance.Snapshot()

	if snap == nil {
		return empty
	}

	cash := "0.00"
	unrealized := "0.00"
	equity := "0.00"

	if snap.Cash != nil {
		cash = snap.Cash.String()
	}

	if snap.Unrealized != nil {
		unrealized = snap.Unrealized.String()
	}

	if snap.Equity != nil {
		equity = snap.Equity.String()
	}

	if snap.Equity == nil {
		equity = cash
	}

	return &wire.EquityFrameT{
		Cash:       cash,
		Unrealized: unrealized,
		Equity:     equity,
	}
}

func (trader *Trader) PositionsWire() *wire.PositionsFrameT {
	if trader == nil {
		return &wire.PositionsFrameT{Rows: []*wire.PositionT{}}
	}

	trader.mu.Lock()
	regs := make(map[string]*position.Regulator, len(trader.open))

	for symbol, reg := range trader.open {
		regs[symbol] = reg
	}

	trader.mu.Unlock()

	rows := make([]*wire.PositionT, 0, len(regs))

	for symbol, reg := range regs {
		if reg == nil {
			continue
		}

		entryPrice := reg.Price()
		volume := reg.Volume()
		returnPct := 0.0
		entryPriceStr := ""
		volumeStr := ""
		markStr := ""
		pnlStr := "0.0000"

		if entryPrice != nil {
			entryPriceStr = entryPrice.String()
			markStr = entryPriceStr
		}

		if volume != nil {
			volumeStr = volume.String()
		}

		if trader.price != nil {
			if mark := trader.price.CurrentMark(symbol); mark != nil {
				markStr = mark.String()
			}

			if pnl := trader.price.PnL(symbol, reg); pnl != nil {
				pnlStr = pnl.String()
			}

			returnPct = trader.price.ReturnPct(symbol, reg)
		}

		entryAtNs := reg.EntryAt.UnixNano()

		if entryAtNs <= 0 {
			entryAtNs = time.Now().UnixNano()
		}

		orderID := reg.OrderID

		if orderID == "" {
			orderID = reg.PositionID
		}

		rows = append(rows, &wire.PositionT{
			Status: reg.Status(),
			Decision: &wire.DecisionT{
				Id:         orderID,
				Symbol:     symbol,
				Action:     "enter",
				Confidence: 1.0,
				At:         entryAtNs,
			},
			Holding: &wire.HoldingT{
				Status:      reg.Status(),
				Symbol:      symbol,
				Asset:       symbol,
				Qty:         volumeStr,
				SellableQty: volumeStr,
				EntryAt:     entryAtNs,
				EntryPrice:  entryPriceStr,
				Mark:        markStr,
				Pnl:         pnlStr,
				ReturnPct:   returnPct,
			},
		})
	}

	return &wire.PositionsFrameT{Rows: rows}
}
