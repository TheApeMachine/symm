package broker

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

type Desk struct {
	status         types.Status
	api            *websocket.API
	instrument     *Instrument
	price          *Price
	balance        *Balance
	positions      *sync.Map
	maxPositions   int
	maxReserved    int
	publishPending atomic.Bool
	bound          atomic.Bool
}

func NewDesk(
	api *websocket.API,
	instrument *Instrument,
	price *Price,
	balance *Balance,
) *Desk {
	return &Desk{
		status:       types.INITIALIZING,
		api:          api,
		instrument:   instrument,
		price:        price,
		balance:      balance,
		positions:    &sync.Map{},
		maxPositions: viper.GetViper().GetInt("trading.slots.normal"),
		maxReserved:  viper.GetViper().GetInt("trading.slots.reserved"),
	}
}

/*
Initialize binds desk-level order/execution/ticker handlers once and
subscribes to executions with catch-up snapshots so reconnect can recover.
*/
func (desk *Desk) Initialize() error {
	errnie.Info("initializing desk")
	desk.bind()

	if err := desk.api.SubscribeExecutions(); err != nil {
		desk.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to subscribe to executions",
			err,
		))
	}

	desk.status = types.READY
	return nil
}

func (desk *Desk) Status() types.Status {
	return desk.status
}

/*
bind registers the single fan-out handlers used by every live Position. Per
position On registration was unbounded and raced under concurrent Buy.
*/
func (desk *Desk) bind() {
	if desk.api == nil || !desk.bound.CompareAndSwap(false, true) {
		return
	}

	desk.api.On("add_order", desk.orderAck)
	desk.api.On("executions", desk.executionAck)
	desk.api.On("ticker", desk.tickerAck)
}

func (desk *Desk) orderAck(buf []byte) {
	desk.positions.Range(func(_, value any) bool {
		position := value.(*Position)
		position.OrderAck(buf)

		if position.Status() == types.ERROR {
			desk.evict(position.pair.Symbol)
		}

		return true
	})
}

func (desk *Desk) executionAck(buf []byte) {
	desk.positions.Range(func(_, value any) bool {
		position := value.(*Position)
		position.ExecutionAck(buf)

		status := position.Status()

		if status == types.CLOSED || status == types.ERROR {
			desk.evict(position.pair.Symbol)
		}

		return true
	})
}

func (desk *Desk) tickerAck(buf []byte) {
	desk.positions.Range(func(_, value any) bool {
		position := value.(*Position)

		if position.Status() == types.CLOSED || position.Status() == types.ERROR {
			return true
		}

		position.TickerAck(buf)
		return true
	})
}

func (desk *Desk) evict(symbol string) {
	if symbol == "" {
		return
	}

	desk.positions.Delete(symbol)
}

/*
OpenPositions counts live (non-terminal) positions that still occupy a slot.
*/
func (desk *Desk) OpenPositions() int {
	count := 0

	desk.positions.Range(func(_, value any) bool {
		status := value.(*Position).Status()

		if status == types.CLOSED || status == types.ERROR {
			return true
		}

		count++
		return true
	})

	return count
}

/*
Holdings yields open inventory currently managed by the desk so Decide can seed
continuation/exit against broker truth rather than an empty Thesis.
*/
func (desk *Desk) Holdings() []types.Holding {
	if desk == nil || desk.balance == nil {
		return nil
	}

	rows := make([]types.Holding, 0)

	desk.positions.Range(func(key, value any) bool {
		position := value.(*Position)
		status := position.Status()

		if status == types.CLOSED || status == types.ERROR {
			return true
		}

		holding, err := desk.balance.Holding(key.(string))

		if err != nil {
			return true
		}

		if holding.Order == nil {
			holding.Order = &spot.Order{
				Description: &spot.OrderDescription{
					Pair: holding.Symbol,
					Type: "open",
				},
			}
		}

		rows = append(rows, holding)
		return true
	})

	return rows
}

func (desk *Desk) Buy(
	holding types.Holding,
	opportunity bool,
) error {
	desk.bind()
	openPositions := desk.OpenPositions()

	if openPositions >= desk.maxPositions+desk.maxReserved {
		return errnie.Error(errnie.Err(
			errnie.Forbidden,
			"desk at max positions and reserved",
			nil,
		))
	}

	if openPositions >= desk.maxPositions && !opportunity {
		return errnie.Error(errnie.Err(
			errnie.Forbidden,
			"desk at max positions",
			nil,
		))
	}

	if _, exists := desk.positions.Load(holding.Symbol); exists {
		return errnie.Error(errnie.Err(
			errnie.Forbidden,
			"position already exists for "+holding.Symbol,
			nil,
		))
	}

	pair, err := desk.instrument.Pair(holding.Symbol)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get instrument pair for "+holding.Symbol,
			err,
		))
	}

	position := NewPosition(
		desk.api,
		desk.instrument,
		desk.price,
		desk.balance,
		&pair,
	)
	position.onTerminal = desk.evict

	desk.balance.holdings.Store(holding.Symbol, &holding)
	desk.positions.Store(holding.Symbol, position)

	if err := position.Enter(); err != nil {
		desk.balance.holdings.Delete(holding.Symbol)
		desk.positions.Delete(holding.Symbol)
		return err
	}

	return nil
}

/*
Sell submits the selected exit through the Position already associated with
the order symbol, preserving one lifecycle across entry and exit.
*/
func (desk *Desk) Sell(holding types.Holding) error {
	position, ok := desk.positions.Load(holding.Symbol)

	if !ok || position == nil {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"position not found for "+holding.Symbol,
			nil,
		))
	}

	return position.(*Position).Exit()
}

func (desk *Desk) Close() error {
	return nil
}

func (desk *Desk) HasSlot(opportunity bool) bool {
	openPositions := desk.OpenPositions()

	if openPositions >= desk.maxPositions+desk.maxReserved {
		return false
	}

	if openPositions >= desk.maxPositions && !opportunity {
		return false
	}

	return true
}

/*
MaxSlots returns the configured normal plus reserved capacity Decide may compete
for. It is the ceiling, not the currently free count.
*/
func (desk *Desk) MaxSlots() int {
	if desk == nil {
		return 0
	}

	return desk.maxPositions + desk.maxReserved
}

/*
NormalSlots returns the non-opportunity inventory ceiling.
*/
func (desk *Desk) NormalSlots() int {
	if desk == nil {
		return 0
	}

	return desk.maxPositions
}

/*
ReservedSlots returns overflow capacity that only opportunity entries may use
once normal slots are full — the OXT-style ignition lane.
*/
func (desk *Desk) ReservedSlots() int {
	if desk == nil {
		return 0
	}

	return desk.maxReserved
}

/*
Regulate feeds each open position's Stoploss from Thesis logic Evidence and
appends exit Decisions when stop or take_profit fires. Trade remains the sole
order submission path so exits stay auditable on the Thesis.
*/
func (desk *Desk) Regulate(thesis *types.Thesis) {
	if desk == nil || thesis == nil || desk.balance == nil {
		return
	}

	desk.positions.Range(func(key, value any) bool {
		position := value.(*Position)
		status := position.Status()

		if status == types.CLOSED || status == types.ERROR || status == types.PENDING {
			return true
		}

		holding, err := desk.balance.Holding(key.(string))

		if err != nil {
			return true
		}

		evidence := strategy.Project(thesis, holding)
		verdict := position.Regulate(evidence)

		if verdict.Action != "stop" && verdict.Action != "take_profit" {
			return true
		}

		quantity := 0.0

		if holding.Qty != nil {
			quantity = holding.Qty.Float64()
		}

		thesis.Decisions = append(thesis.Decisions, types.Decision{
			Action:           "exit",
			Symbol:           holding.Symbol,
			At:               time.Now().UTC(),
			Utility:          verdict.StopReturn,
			Alternatives:     map[string]float64{verdict.Action: verdict.StopReturn},
			ProposedQuantity: quantity,
			ReferencePrice:   evidence.Mark,
			Cause:            verdict.Action,
			Reason:           verdict.Reason,
		})
		thesis.Lifecycle.Store(holding.Symbol, types.LifecycleExitSelected)

		return true
	})
}
