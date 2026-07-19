package broker

import (
	"slices"
	"sync"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
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
	marks          Marks
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
	positions := &sync.Map{}

	return &Desk{
		status:       types.INITIALIZING,
		api:          api,
		instrument:   instrument,
		price:        price,
		balance:      balance,
		positions:    positions,
		marks:        Marks{positions: positions},
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

	if desk.price != nil {
		desk.price.RouteMarks(desk.Mark)
	}

	if err := desk.api.SubscribeExecutions(); err != nil {
		desk.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to subscribe to executions",
			err,
		))
	}

	desk.adoptOpen()
	desk.status = types.READY

	if desk.balance != nil {
		desk.balance.Publish()
	}

	return nil
}

/*
adoptOpen wraps Balance-seeded restart inventory in Position shells so ticker
marks and exits can reach recovered lots. Safe to call again after instrument
metadata arrives — early Initialize often runs before Pair lookups succeed.
*/
func (desk *Desk) adoptOpen() {
	if desk == nil || desk.balance == nil || desk.instrument == nil {
		return
	}

	for holding := range desk.balance.Holdings() {
		if _, exists := desk.positions.Load(holding.Symbol); exists {
			continue
		}

		pair, err := desk.instrument.Pair(holding.Symbol)

		if err != nil {
			continue
		}

		position := NewPosition(
			desk.api,
			desk.instrument,
			desk.price,
			desk.balance,
			pair,
		)
		position.status = types.OPEN
		position.onTerminal = desk.evict
		lot := holding
		position.TakeStop(&lot)
		desk.balance.holdings.Store(holding.Symbol, &lot)
		desk.positions.Store(holding.Symbol, position)
	}
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
			types.CLOSED, types.ERROR, types.CANCELED,
		}, position.Status()) {
			desk.evict(position.pair.Symbol)
			return true
		}

		// Wallet Balance is inventory authority; Position status can lag a
		// syncWallet flatten until the next execution print.
		if desk.balance == nil || position.pair == nil {
			return true
		}

		holding, err := desk.balance.Holding(position.pair.Symbol)

		if err != nil || holding.Status != types.CLOSED {
			return true
		}

		position.status = types.CLOSED
		desk.evict(position.pair.Symbol)

		return true
	})
}

/*
OpenPositions counts open inventory that occupies a slot. Wallet holdings are
the source of truth — position shells can lag adoptOpen or disappear on ERROR
while paper balances remain.
*/
func (desk *Desk) OpenPositions() int {
	desk.Update()
	desk.adoptOpen()

	if desk.balance == nil {
		return 0
	}

	count := 0

	for range desk.balance.Holdings() {
		count++
	}

	return count
}

/*
Position returns the live position shell for symbol, adopting restart inventory
when the instrument registry can resolve the pair.
*/
func (desk *Desk) Position(symbol string) (*Position, bool) {
	if desk == nil || symbol == "" {
		return nil, false
	}

	desk.adoptOpen()
	value, ok := desk.positions.Load(symbol)

	if !ok {
		return nil, false
	}

	position, ok := value.(*Position)

	return position, ok && position != nil
}

/*
Pending reports whether symbol already has an outstanding broker order.
*/
func (desk *Desk) Pending(symbol string) bool {
	position, ok := desk.Position(symbol)

	return ok && desk.marks.Pending(position)
}

/*
PendingSymbols returns outstanding entry/exit/reduce intents for recovery.
*/
func (desk *Desk) PendingSymbols() map[string]types.PendingOrderWire {
	if desk == nil {
		return map[string]types.PendingOrderWire{}
	}

	return desk.marks.PendingSymbols()
}

func (desk *Desk) Buy(
	holding types.Holding,
	opportunity bool,
) (*Position, error) {
	return desk.BuyAfter(holding, opportunity, 0, "")
}

/*
BuyAfter places an entry. freeing is retained for Arbiter selection probes; the
trade path passes zero so challenger buys wait for real inventory exits (fill
gate) rather than optimistic sell-submit credit.
*/
func (desk *Desk) BuyAfter(
	holding types.Holding,
	opportunity bool,
	freeing int,
	reservationID string,
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
	position.claim.Bind(desk.balance, reservationID)

	seed := holding
	position.TakeStop(&seed)
	desk.balance.holdings.Store(holding.Symbol, &seed)
	desk.positions.Store(holding.Symbol, position)

	if err := position.Enter(); err != nil {
		position.claim.Release()
		desk.evict(holding.Symbol)

		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to enter position",
			err,
		))
	}

	return position, nil
}

/*
Mark applies a Price-cache mark to an open lot after a single ticker decode.
*/
func (desk *Desk) Mark(symbol string) {
	if desk == nil {
		return
	}

	desk.marks.Apply(symbol)
}

/*
Sell submits a full exit (nil quantity) or a partial reduce for a desk-owned lot.
*/
func (desk *Desk) Sell(symbol string, quantity *decimal.Decimal) error {
	position, ok := desk.Position(symbol)

	if !ok {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"desk: no open position for "+symbol,
			nil,
		))
	}

	return position.Sell(quantity)
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
SetSlots overrides normal and reserved capacity. Fixtures use this so parallel
tests do not race on process-wide viper values after Desk construction.
*/
func (desk *Desk) SetSlots(normal, reserved int) {
	if desk == nil {
		return
	}

	desk.maxPositions = normal
	desk.maxReserved = reserved
}

/*
Free returns free normal and reserved slot counts given how many positions are
already open. Desk owns the slot arithmetic so Planner never re-derives it.
*/
func (desk *Desk) Free(open int) (normal, reserved int) {
	if desk == nil {
		return 0, 0
	}

	normal = max(0, desk.NormalSlots()-open)
	reserved = max(0, desk.MaxSlots()-open-normal)

	return normal, reserved
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
