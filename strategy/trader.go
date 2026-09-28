package strategy

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
Trader is responsible for talking to the broker and managing positions.
*/
type Trader struct {
	*runtime.System
	desk      *broker.Desk
	balance   *broker.Balance
	price     *broker.Price
	positions sync.Map
	decisions atomic.Pointer[[]*wire.DecisionT]
}

func NewTrader(
	ctx context.Context,
	api *websocket.API,
	price *broker.Price,
	balance *broker.Balance,
) *Trader {
	trader := &Trader{
		System:  runtime.NewSystem(ctx, "trader"),
		desk:    broker.NewDesk(ctx, api, price, balance),
		balance: balance,
		price:   price,
	}

	initialDecisions := make([]*wire.DecisionT, 0, 50)
	trader.decisions.Store(&initialDecisions)

	if api != nil {
		api.OnExecution(trader.ApplyExecution)
	}

	return trader
}

func (trader *Trader) OnAction(symbol string, action Action) {
	if trader == nil || symbol == "" {
		return
	}

	switch action {
	case ActionEnter:
		if _, exists := trader.positions.Load(symbol); exists {
			return
		}

		plannerConfig := system.NewPlannerConfig()

		if trader.price != nil {
			var totalCost *decimal.Decimal
			var totalPnL *decimal.Decimal

			trader.positions.Range(func(key, value any) bool {
				symbolKey, okKey := key.(string)
				positionVal, okVal := value.(*broker.Position)

				if !okKey || !okVal || positionVal == nil {
					return true
				}

				pnl := trader.price.PnL(symbolKey, positionVal)

				if pnl != nil {
					if totalPnL == nil {
						totalPnL = pnl
					} else {
						totalPnL = totalPnL.Add(pnl)
					}
				}

				entryPrice := positionVal.Price()
				entryVolume := positionVal.Volume()

				if entryPrice != nil && entryVolume != nil {
					cost := entryPrice.Mul(entryVolume)

					if totalCost == nil {
						totalCost = cost
					} else {
						totalCost = totalCost.Add(cost)
					}
				}

				return true
			})

			if totalCost != nil && totalCost.Sign() > 0 && totalPnL != nil && totalPnL.Sign() < 0 {
				lossRatio := totalPnL.Abs().Div(totalCost).Float64()

				if lossRatio >= plannerConfig.AggregateMaxLossFraction {
					trader.RecordDecision(symbol, "blocked", 0.0, "aggregate max loss fraction exceeded")
					return
				}
			}
		}

		position := trader.desk.Enter(symbol)

		if position != nil {
			trader.positions.Store(symbol, position)
			trader.RecordDecision(symbol, "enter", 1.0, "precursor trigger")
		}
	case ActionExit:
		val, found := trader.positions.Load(symbol)

		if found && val != nil {
			position, ok := val.(*broker.Position)

			if !ok || position == nil {
				trader.positions.Delete(symbol)
				return
			}

			if err := trader.desk.Exit(position); err != nil {
				trader.RecordDecision(symbol, "exit_failed", 1.0, fmt.Sprintf("exit failed: %v", err))
				return
			}

			trader.positions.Delete(symbol)
			trader.RecordDecision(symbol, "exit", 1.0, "exit trigger")
		}
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
			break
		}
	}
}

func (trader *Trader) Position(symbol string) *broker.Position {
	if trader == nil {
		return nil
	}

	val, ok := trader.positions.Load(symbol)

	if !ok || val == nil {
		return nil
	}

	position, _ := val.(*broker.Position)
	return position
}

func (trader *Trader) Holding(symbol string) bool {
	if trader == nil {
		return false
	}

	val, ok := trader.positions.Load(symbol)
	return ok && val != nil
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

func (trader *Trader) Positions() map[string]*broker.Position {
	if trader == nil {
		return nil
	}

	snapshot := make(map[string]*broker.Position)
	trader.positions.Range(func(key, val any) bool {
		symbolKey, okKey := key.(string)
		posVal, okVal := val.(*broker.Position)

		if okKey && okVal && posVal != nil {
			snapshot[symbolKey] = posVal
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

func (trader *Trader) PositionsWire() *wire.PositionsFrameT {
	if trader == nil {
		return &wire.PositionsFrameT{Rows: []*wire.PositionT{}}
	}

	rows := make([]*wire.PositionT, 0)

	trader.positions.Range(func(key, val any) bool {
		symbolKey, okKey := key.(string)
		position, okPos := val.(*broker.Position)

		if !okKey || !okPos || position == nil {
			return true
		}

		entryPrice := position.Price()
		volume := position.Volume()
		var mark *decimal.Decimal
		var pnl *decimal.Decimal
		returnPct := 0.0

		if trader.price != nil {
			mark = trader.price.CurrentMark(symbolKey)
			pnl = trader.price.PnL(symbolKey, position)
			returnPct = trader.price.ReturnPct(symbolKey, position)
		}

		entryPriceStr := ""

		if entryPrice != nil {
			entryPriceStr = entryPrice.String()
		}

		volumeStr := ""

		if volume != nil {
			volumeStr = volume.String()
		}

		markStr := entryPriceStr

		if mark != nil {
			markStr = mark.String()
		}

		pnlStr := "0.0000"

		if pnl != nil {
			pnlStr = pnl.String()
		}

		entryAtNs := position.EntryAt.UnixNano()

		if entryAtNs <= 0 {
			entryAtNs = time.Now().UnixNano()
		}

		holding := &wire.HoldingT{
			Status:      "active",
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

		decision := &wire.DecisionT{
			Id:         position.OrderID(),
			Symbol:     symbolKey,
			Action:     "enter",
			Confidence: 1.0,
			At:         entryAtNs,
		}

		rows = append(rows, &wire.PositionT{
			Status:   "active",
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
		return &wire.StrategyFrameT{Evaluated: true, Decisions: []*wire.DecisionT{}}
	}

	decPtr := trader.decisions.Load()
	var decisions []*wire.DecisionT

	if decPtr != nil {
		decisions = make([]*wire.DecisionT, len(*decPtr))
		copy(decisions, *decPtr)
	}

	return &wire.StrategyFrameT{
		Evaluated: true,
		Outcome:   "active",
		Decisions: decisions,
	}
}

func (trader *Trader) ApplyExecution(exec *kraken.Execution) {
	if trader == nil || exec == nil {
		return
	}

	for _, item := range exec.Data {
		var pos *broker.Position

		if val, ok := trader.positions.Load(item.Symbol); ok && val != nil {
			pos, _ = val.(*broker.Position)
		}

		if pos == nil {
			trader.positions.Range(func(_, val any) bool {
				candidate, okCandidate := val.(*broker.Position)

				if okCandidate && candidate != nil && (candidate.OrderID() == item.OrderID || candidate.OrderID() == item.ClientOrderID) {
					pos = candidate
					return false
				}

				return true
			})
		}

		if pos == nil {
			continue
		}

		fillPrice := item.AvgPrice

		if fillPrice == nil || fillPrice.Sign() <= 0 {
			fillPrice = item.LastPrice
		}

		fillVolume := item.CumQty

		if fillVolume == nil || fillVolume.Sign() <= 0 {
			fillVolume = item.LastQty
		}

		fee := item.FeeUsdEquiv

		if fee == nil && len(item.Fees) > 0 {
			fee = decimal.NewFromFloat64(item.Fees[0].Qty)
		}

		if fillPrice != nil && fillVolume != nil {
			pos.SetFill(fillPrice, fillVolume, fee)
		}
	}
}
