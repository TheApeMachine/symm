package strategy

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

type PositionCloseCallback func(symbol string, returnFraction float64, fee float64)

/*
Trader is responsible for talking to the broker and managing positions.
*/
type Trader struct {
	*runtime.System
	desk             *broker.Desk
	balance          *broker.Balance
	price            *broker.Price
	positions        sync.Map
	symbolLocks      sync.Map
	positionsVersion atomic.Uint64
	decisionsVersion atomic.Uint64
	decisions        atomic.Pointer[[]*wire.DecisionT]
	admissionMu      sync.Mutex
	reservedCash     *decimal.Decimal
	onPositionClosed PositionCloseCallback
}

func (trader *Trader) symbolLock(symbol string) *sync.Mutex {
	val, _ := trader.symbolLocks.LoadOrStore(symbol, &sync.Mutex{})
	return val.(*sync.Mutex)
}

func NewTrader(
	ctx context.Context,
	private *network.WebsocketClient,
	price *broker.Price,
	balance *broker.Balance,
) *Trader {
	trader := &Trader{
		System:       runtime.NewSystem(ctx, "trader"),
		desk:         broker.NewDesk(ctx, private, price, balance),
		balance:      balance,
		price:        price,
		reservedCash: decimal.NewFromInt64(0).SetScale(decimal.DefaultScale),
	}

	trader.positionsVersion.Store(1)
	trader.decisionsVersion.Store(1)

	initialDecisions := make([]*wire.DecisionT, 0, 50)
	trader.decisions.Store(&initialDecisions)

	return trader
}

func (trader *Trader) OnAction(symbol string, action cognition.Action) {
	if trader == nil || symbol == "" {
		return
	}

	mu := trader.symbolLock(symbol)
	mu.Lock()
	defer mu.Unlock()

	// Hard safety check: staged training only ever authorizes paper execution
	if system.Cfg.Market.Model != "paper" {
		trader.RecordDecision(symbol, "blocked", 0.0, "real money execution disabled in staged training")
		return
	}

	switch action {
	case cognition.ActionEnter:
		if _, exists := trader.positions.Load(symbol); exists {
			return
		}

		plannerConfig := system.NewPlannerConfig()

		trader.admissionMu.Lock()

		if _, exists := trader.positions.Load(symbol); exists {
			trader.admissionMu.Unlock()
			return
		}

		if trader.price != nil {
			var totalCost *decimal.Decimal
			var totalPnL *decimal.Decimal

			trader.positions.Range(func(key, value any) bool {
				symbolKey, okKey := key.(string)
				reg, okVal := value.(*position.Regulator)

				if !okKey || !okVal || reg == nil {
					return true
				}

				pnl := trader.price.PnL(symbolKey, reg)

				if pnl != nil {
					if totalPnL == nil {
						totalPnL = pnl
					}

					if totalPnL != pnl {
						totalPnL = safeAdd(totalPnL, pnl)
					}
				}

				if reg.Basis != nil && reg.Basis.Sign() > 0 {
					if totalCost == nil {
						totalCost = reg.Basis
					}

					if totalCost != reg.Basis {
						totalCost = safeAdd(totalCost, reg.Basis)
					}
				}

				return true
			})

			if totalCost != nil && totalCost.Sign() > 0 && totalPnL != nil && totalPnL.Sign() < 0 {
				lossRatio := totalPnL.Abs().Div(totalCost).Float64()

				if lossRatio >= plannerConfig.AggregateMaxLossFraction {
					trader.admissionMu.Unlock()
					trader.RecordDecision(symbol, "blocked", 0.0, "aggregate max loss fraction exceeded")
					return
				}
			}
		}

		maxFraction := viper.GetFloat64("trading.allocation.max_fraction")

		if maxFraction <= 0 || maxFraction > 1 {
			trader.admissionMu.Unlock()
			return
		}

		cash := trader.balance.Cash()

		if cash == nil || cash.Sign() <= 0 {
			trader.balance.Update()
			cash = trader.balance.Cash()
		}

		if cash == nil || cash.Sign() <= 0 {
			trader.admissionMu.Unlock()
			trader.RecordDecision(symbol, "blocked", 0.0, "insufficient cash")
			return
		}

		availableCash := safeSub(cash, trader.reservedCash)

		if availableCash == nil || availableCash.Sign() <= 0 {
			trader.admissionMu.Unlock()
			trader.RecordDecision(symbol, "blocked", 0.0, "insufficient unreserved cash")
			return
		}

		spend := availableCash.SetScale(decimal.DefaultScale).Mul(decimal.NewFromFloat64(maxFraction))
		trader.reservedCash = safeAdd(trader.reservedCash, spend)

		reg := position.NewRegulator(symbol)
		trader.positions.Store(symbol, reg)
		trader.positionsVersion.Add(1)
		trader.admissionMu.Unlock()

		defer func() {
			trader.admissionMu.Lock()
			trader.reservedCash = safeSub(trader.reservedCash, spend)

			if trader.reservedCash == nil || trader.reservedCash.Sign() < 0 {
				trader.reservedCash = decimal.NewFromInt64(0).SetScale(decimal.DefaultScale)
			}

			trader.admissionMu.Unlock()
		}()

		if err := trader.desk.Execution.EnterWithRegulator(reg, spend); err != nil {
			trader.positions.Delete(symbol)
			trader.positionsVersion.Add(1)
			trader.RecordDecision(symbol, "blocked", 0.0, fmt.Sprintf("enter failed: %v", err))
			return
		}

		trader.RecordDecision(symbol, "enter", 1.0, "precursor trigger")
	case cognition.ActionExit:
		val, found := trader.positions.Load(symbol)

		if !found || val == nil {
			return
		}

		reg, ok := val.(*position.Regulator)

		if !ok || reg == nil {
			trader.positions.Delete(symbol)
			trader.positionsVersion.Add(1)
			return
		}

		if !reg.IsHolding() {
			return
		}

		if reg.Status() == "exit_pending" {
			return
		}

		if err := trader.desk.Execution.Exit(reg); err != nil {
			trader.RecordDecision(symbol, "exit_failed", 1.0, fmt.Sprintf("exit failed: %v", err))
			return
		}

		trader.RecordDecision(symbol, "exit", 1.0, "exit trigger")
	}
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
		var oldSlice []*wire.DecisionT

		if oldPtr != nil {
			oldSlice = *oldPtr
		}

		newSlice := make([]*wire.DecisionT, 0, len(oldSlice)+1)
		startIndex := 0

		if len(oldSlice) >= 50 {
			startIndex = len(oldSlice) - 49
		}

		newSlice = append(newSlice, oldSlice[startIndex:]...)
		newSlice = append(newSlice, decision)

		if trader.decisions.CompareAndSwap(oldPtr, &newSlice) {
			trader.decisionsVersion.Add(1)
			break
		}
	}
}

func (trader *Trader) Position(symbol string) *position.Regulator {
	if trader == nil {
		return nil
	}

	val, ok := trader.positions.Load(symbol)

	if !ok || val == nil {
		return nil
	}

	reg, _ := val.(*position.Regulator)
	return reg
}

func (trader *Trader) Holding(symbol string) bool {
	if trader == nil {
		return false
	}

	val, ok := trader.positions.Load(symbol)

	if !ok || val == nil {
		return false
	}

	reg, ok := val.(*position.Regulator)
	return ok && reg != nil && reg.IsHolding()
}

func (trader *Trader) HasFilledPosition(symbol string) bool {
	if trader == nil {
		return false
	}

	val, ok := trader.positions.Load(symbol)
	if !ok || val == nil {
		return false
	}

	reg, ok := val.(*position.Regulator)
	return ok && reg != nil && reg.IsHolding()
}

func (trader *Trader) PositionCount() int {
	if trader == nil {
		return 0
	}

	count := 0
	trader.positions.Range(func(_, val any) bool {
		if val != nil {
			count++
		}

		return true
	})

	return count
}

func (trader *Trader) Positions() map[string]*position.Regulator {
	if trader == nil {
		return nil
	}

	snapshot := make(map[string]*position.Regulator)
	trader.positions.Range(func(key, val any) bool {
		symbolKey, okKey := key.(string)
		regVal, okVal := val.(*position.Regulator)

		if okKey && okVal && regVal != nil {
			snapshot[symbolKey] = regVal
		}

		return true
	})

	return snapshot
}

func (trader *Trader) Balance() *broker.Balance {
	if trader == nil {
		return nil
	}

	return trader.balance
}

func (trader *Trader) EquityWire() *wire.EquityFrameT {
	if trader == nil || trader.balance == nil {
		return &wire.EquityFrameT{
			Cash:       "0.00",
			Unrealized: "0.00",
			Equity:     "0.00",
		}
	}

	snap := trader.balance.Snapshot()

	if snap == nil {
		return &wire.EquityFrameT{
			Cash:       "0.00",
			Unrealized: "0.00",
			Equity:     "0.00",
		}
	}

	cashStr := "0.00"

	if snap.Cash != nil {
		cashStr = snap.Cash.String()
	}

	unrealizedStr := "0.00"

	if snap.Unrealized != nil {
		unrealizedStr = snap.Unrealized.String()
	}

	equityStr := cashStr

	if snap.Equity != nil {
		equityStr = snap.Equity.String()
	}

	return &wire.EquityFrameT{
		Cash:       cashStr,
		Unrealized: unrealizedStr,
		Equity:     equityStr,
	}
}

func (trader *Trader) PositionsWire() *wire.PositionsFrameT {
	if trader == nil {
		return &wire.PositionsFrameT{Rows: []*wire.PositionT{}}
	}

	rows := make([]*wire.PositionT, 0)

	trader.positions.Range(func(key, val any) bool {
		symbolKey, okKey := key.(string)
		reg, okPos := val.(*position.Regulator)

		if !okKey || !okPos || reg == nil {
			return true
		}

		entryPrice := reg.Price()
		volume := reg.Volume()
		var mark *decimal.Decimal
		var pnl *decimal.Decimal
		returnPct := 0.0

		if trader.price != nil {
			mark = trader.price.CurrentMark(symbolKey)
			pnl = trader.price.PnL(symbolKey, reg)
			returnPct = trader.price.ReturnPct(symbolKey, reg)
		}

		entryPriceStr := ""

		if entryPrice != nil {
			entryPriceStr = entryPrice.String()
		}

		volumeStr := ""

		if volume != nil {
			volumeStr = volume.String()
		}

		markStr := ""

		if mark != nil {
			markStr = mark.String()
		}

		pnlStr := ""

		if pnl != nil {
			pnlStr = pnl.String()
		}

		var entryAtNs int64

		if !reg.EntryAt.IsZero() {
			entryAtNs = reg.EntryAt.UnixNano()
		}

		orderID := reg.OrderID

		if orderID == "" {
			orderID = reg.PositionID
		}

		holding := &wire.HoldingT{
			Status:      reg.Status(),
			Symbol:      symbolKey,
			Asset:       symbolKey,
			Qty:         volumeStr,
			SellableQty: volumeStr,
			EntryAt:     entryAtNs,
			EntryPrice:  entryPriceStr,
			Mark:        markStr,
			Pnl:         pnlStr,
			ReturnPct:   returnPct,
		}

		var decision *wire.DecisionT

		if orderID != "" {
			decision = &wire.DecisionT{
				Id:         orderID,
				Symbol:     symbolKey,
				Action:     "enter",
				Confidence: 0.0,
				At:         entryAtNs,
			}
		}

		rows = append(rows, &wire.PositionT{
			Status:   reg.Status(),
			Decision: decision,
			Holding:  holding,
		})

		return true
	})

	return &wire.PositionsFrameT{Rows: rows}
}

func (trader *Trader) RecentTrades(limit int) ([]*wire.PositionT, error) {
	wireFrame := trader.PositionsWire()

	if wireFrame == nil {
		return []*wire.PositionT{}, nil
	}

	if limit > 0 && len(wireFrame.Rows) > limit {
		return wireFrame.Rows[:limit], nil
	}

	return wireFrame.Rows, nil
}

func (trader *Trader) DecisionsWire() *wire.StrategyFrameT {
	if trader == nil {
		return &wire.StrategyFrameT{Evaluated: false, Decisions: []*wire.DecisionT{}}
	}

	decPtr := trader.decisions.Load()
	var decisions []*wire.DecisionT

	if decPtr != nil {
		decisions = make([]*wire.DecisionT, len(*decPtr))
		copy(decisions, *decPtr)
	}

	outcome := ""

	if len(decisions) > 0 {
		outcome = "active"
	}

	return &wire.StrategyFrameT{
		Evaluated: len(decisions) > 0,
		Outcome:   outcome,
		Decisions: decisions,
	}
}

func (trader *Trader) ApplyExecution(exec *kraken.Execution) {
	if trader == nil || exec == nil {
		return
	}

	for _, item := range exec.Data {
		var matched *position.Regulator

		// Authoritative match: Check order ID or client order ID
		trader.positions.Range(func(_, val any) bool {
			reg, ok := val.(*position.Regulator)

			if ok && reg != nil && reg.Identifies(item.OrderID, item.ClientOrderID) {
				matched = reg
				return false
			}

			return true
		})

		// Fallback only if single position for symbol and order IDs match symbol
		if matched == nil && item.Symbol != "" {
			if val, ok := trader.positions.Load(item.Symbol); ok && val != nil {
				reg, _ := val.(*position.Regulator)

				if reg != nil && (reg.OrderID == "" || reg.OrderID == item.OrderID) {
					matched = reg
				}
			}
		}

		if matched == nil {
			continue
		}

		mu := trader.symbolLock(matched.Symbol)
		mu.Lock()

		if err := matched.Reconcile(item); err != nil {
			errnie.Error(err)
			mu.Unlock()
			continue
		}

		trader.positionsVersion.Add(1)

		if matched.IsClosed() {
			closedCost := matched.ClosedCost()

			if trader.onPositionClosed != nil && closedCost != nil && closedCost.Sign() > 0 && matched.Realized != nil {
				returnFrac := matched.Realized.Div(closedCost).Float64()
				fee := 0.0

				if feeDec := matched.ClosedFee(); feeDec != nil {
					fee = feeDec.Float64()
				}

				trader.onPositionClosed(matched.Symbol, returnFrac, fee)
			}

			trader.positions.Delete(matched.Symbol)
			trader.positionsVersion.Add(1)
		}

		mu.Unlock()
	}
}

func (trader *Trader) SetOnPositionClosed(fn PositionCloseCallback) {
	if trader != nil {
		trader.onPositionClosed = fn
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

func safeSub(a, b *decimal.Decimal) *decimal.Decimal {
	if a == nil {
		return nil
	}

	if b == nil {
		return a
	}

	scale := max(a.GetScale(), b.GetScale())

	if scale < decimal.DefaultScale {
		scale = decimal.DefaultScale
	}

	return a.SetScale(scale).Sub(b)
}

func safeAdd(a, b *decimal.Decimal) *decimal.Decimal {
	if a == nil {
		return b
	}

	if b == nil {
		return a
	}

	scale := max(a.GetScale(), b.GetScale())

	if scale < decimal.DefaultScale {
		scale = decimal.DefaultScale
	}

	return a.SetScale(scale).Add(b)
}
