package broker

import (
	"slices"
	"sync"
	"sync/atomic"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
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
	onInventory    atomic.Value
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
Initialize binds desk-level order/execution/ticker handlers once,
subscribes to executions, and adopts Balance-seeded holdings from restart.
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

	if desk.balance != nil {
		desk.balance.Publish()
	}

	return nil
}

func (desk *Desk) Status() types.Status {
	return desk.status
}

/*
Update checks all positions and removes any that have been
closed or errored.
*/
func (desk *Desk) Update() {
	desk.positions.Range(func(_, value any) bool {
		position := value.(*Position)

		if slices.Contains([]types.Status{
			types.CLOSED, types.ERROR,
		}, position.Status()) {
			desk.balance.holdings.Delete(position.pair.Symbol)
			desk.positions.Delete(position.pair.Symbol)
		}

		return true
	})
}

/*
OpenPositions counts live (non-terminal) positions that still occupy a slot.
*/
func (desk *Desk) OpenPositions() int {
	desk.Update()
	count := 0

	desk.positions.Range(func(_, value any) bool {
		count++
		return true
	})

	return count
}

func (desk *Desk) Buy(
	holding types.Holding,
	opportunity bool,
) (*Position, error) {
	return desk.BuyAfter(holding, opportunity, 0)
}

/*
BuyAfter places an entry while treating freeing same-tick full exits as already
vacated slots so rotate can sell then buy before the exit fill lands.
*/
func (desk *Desk) BuyAfter(
	holding types.Holding,
	opportunity bool,
	freeing int,
) (*Position, error) {
	desk.bind()
	openPositions := max(desk.OpenPositions()-freeing, 0)

	if openPositions >= desk.maxPositions+desk.maxReserved {
		return nil, errnie.Error(errnie.Err(
			errnie.Forbidden,
			"desk at max positions and reserved",
			nil,
		))
	}

	if openPositions >= desk.maxPositions && !opportunity {
		return nil, errnie.Error(errnie.Err(
			errnie.Forbidden,
			"desk at max positions",
			nil,
		))
	}

	if _, exists := desk.positions.Load(holding.Symbol); exists {
		return nil, errnie.Error(errnie.Err(
			errnie.Forbidden,
			"position already exists for "+holding.Symbol,
			nil,
		))
	}

	pair, err := desk.instrument.Pair(holding.Symbol)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
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
		pair,
	)
	position.onTerminal = desk.evict

	desk.balance.holdings.Store(holding.Symbol, &holding)
	desk.positions.Store(holding.Symbol, position)

	if err := position.Enter(); err != nil {
		desk.balance.holdings.Delete(holding.Symbol)
		desk.positions.Delete(holding.Symbol)

		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to enter position",
			err,
		))
	}

	return position, nil
}

func (desk *Desk) Close() error {
	return nil
}

func (desk *Desk) HasSlot(opportunity bool) bool {
	return desk.HasSlotAfter(opportunity, 0)
}

/*
HasSlotAfter reports whether an enter fits after counting same-tick full exits
as freed capacity — required for rotate to clear the slot ceiling in one tick.
*/
func (desk *Desk) HasSlotAfter(opportunity bool, freeing int) bool {
	openPositions := max(desk.OpenPositions()-freeing, 0)

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
