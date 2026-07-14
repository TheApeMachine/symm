package broker

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

type Desk struct {
	status         types.Status
	api            *websocket.API
	ui             chan []byte
	price          *Price
	balance        *Balance
	positions      *sync.Map
	maxPositions   int
	maxReserved    int
	publishPending atomic.Bool
}

func NewDesk(
	api *websocket.API,
	price *Price,
	balance *Balance,
	messages chan []byte,
) *Desk {
	return &Desk{
		status:       types.INITIALIZING,
		api:          api,
		ui:           messages,
		price:        price,
		balance:      balance,
		positions:    &sync.Map{},
		maxPositions: viper.GetViper().GetInt("trading.slots.normal"),
		maxReserved:  viper.GetViper().GetInt("trading.slots.reserved"),
	}
}

/*
Initialize waits for the wallet to report holdings, then loads open spot
inventory from trade history. The booter calls this once per boot.
*/
func (desk *Desk) Initialize() error {
	errnie.Info("initializing desk")

	history, err := desk.api.TradesHistory()

	if err != nil {
		desk.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get trades history",
			err,
		))
	}

	for _, holding := range desk.balance.Holdings() {
		if holding.Asset == desk.balance.quote || holding.Qty.Sign() <= 0 {
			continue
		}

		fixed := desk.balance.Symbol(holding.Asset)
		data := &PositionData{
			Symbol: fixed,
			Qty:    holding.Qty,
			Mark:   *decimal.NewFromInt64(0),
			PnL:    *decimal.NewFromInt64(0),
		}

		/*
			Ask Price to calculate the complete current position value.

			Desk does not calculate:
			  - notionals
			  - fees
			  - PnL
			  - return percentage

			The ticker universe subscribes hundreds of symbols on boot, so
			this symbol's own ticker may not have arrived yet. That is not
			a reason to drop the holding: Hydrate below still confirms it
			against the wallet, and Position.TickerAck fills in Mark, PnL,
			and ReturnPct the moment this symbol's next ticker arrives.
		*/
		position := NewPosition(
			desk.api,
			desk.ui,
			desk.price,
			desk.balance,
			data,
			types.NewThesis(nil),
			desk.snapshot,
		)

		desk.positions.Store(fixed, position)
		position.Hydrate(fixed, history)

		if position.Status() != types.OPEN {
			desk.positions.Delete(fixed)
		}
	}

	desk.status = types.READY
	desk.Publish()

	return nil
}

/*
snapshot returns the current data for every position desk holds, so a
single position's own publish can report the full open set instead of
just itself.
*/
func (desk *Desk) snapshot() []PositionData {
	positions := make([]PositionData, 0)

	desk.positions.Range(func(_, value any) bool {
		position := value.(*Position)

		if position.Data.Qty.Sign() > 0 {
			positions = append(positions, *position.Data)
		}

		return true
	})

	return positions
}

func (desk *Desk) Publish() {
	payload := desk.marshalSnapshot()

	select {
	case desk.ui <- payload:
	default:
		desk.enqueuePublish()
	}
}

func (desk *Desk) marshalSnapshot() []byte {
	snapshot := datura.Map[any]{
		"positions": desk.snapshot(),
	}
	balances := desk.balance.Snapshot()

	if len(balances) > 0 {
		snapshot["balances"] = balances
	}

	return snapshot.Marshal()
}

func (desk *Desk) enqueuePublish() {
	if !desk.publishPending.CompareAndSwap(false, true) {
		return
	}

	go desk.flushPublish()
}

func (desk *Desk) flushPublish() {
	defer desk.publishPending.Store(false)

	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		payload := desk.marshalSnapshot()

		select {
		case desk.ui <- payload:
			return
		default:
			// ponytail: UI channel backpressure coalesces to the newest snapshot
			// and retries until the hub accepts one frame.
		}
	}
}

func (desk *Desk) Status() types.Status {
	return desk.status
}

func (desk *Desk) OpenPositions() int {
	count := 0

	desk.positions.Range(func(_, value any) bool {
		position := value.(*Position)

		if position.Data.Qty.Sign() > 0 || position.Status() == types.PENDING ||
			position.Status() == types.OPEN {
			count++
		}

		return true
	})

	return count
}

func (desk *Desk) Positions() []*Position {
	positions := make([]*Position, 0)

	desk.positions.Range(func(_, value any) bool {
		position := value.(*Position)

		if position.Data.Qty.Sign() > 0 {
			positions = append(positions, position)
		}

		return true
	})

	return positions
}

/*
Exposures returns the immutable position values and originating Thesis needed
to compare continuation alternatives without exposing mutable Positions.
*/
func (desk *Desk) Exposures() map[string]types.Exposure {
	exposures := make(map[string]types.Exposure)

	desk.positions.Range(func(key, value any) bool {
		position := value.(*Position)

		if position.Data.Qty.Sign() > 0 {
			notional := 0.0

			if position.Data.Mark.Sign() > 0 {
				notional = desk.price.Notional(
					position.Data.Mark, position.Data.Qty,
				).Float64()
			}

			exposures[key.(string)] = types.Exposure{
				Thesis: position.Thesis(), Quantity: position.Data.Qty.Float64(),
				Mark: position.Data.Mark.Float64(), Notional: notional,
				ReturnPct: position.Data.ReturnPct,
			}
		}

		return true
	})

	return exposures
}

/*
PostExit advances closed position Theses with later forecast epochs and returns
only those whose evidence-derived observation tail is complete.
*/
func (desk *Desk) PostExit(current *types.Thesis) map[string]*types.Thesis {
	ready := make(map[string]*types.Thesis)

	desk.positions.Range(func(key, value any) bool {
		position := value.(*Position)
		symbol := key.(string)
		state := position.Thesis().LifecycleState(symbol)

		if state == types.LifecyclePostMortemReady {
			ready[symbol] = position.Thesis()

			return true
		}

		if position.Status() != types.CLOSED ||
			(state != types.LifecycleClosed && state != types.LifecyclePostExitObservation) {
			return true
		}

		if err := position.Thesis().ObservePostExit(current, symbol); err != nil {
			errnie.Error(err)

			return true
		}

		if position.Thesis().LifecycleState(symbol) == types.LifecyclePostMortemReady {
			ready[symbol] = position.Thesis()
		}

		return true
	})

	return ready
}

/*
Finalize removes runtime position state only after PostMortem evaluated the
same Thesis retained by that position.
*/
func (desk *Desk) Finalize(symbol string, thesis *types.Thesis) error {
	value, exists := desk.positions.Load(symbol)

	if !exists || value.(*Position).Thesis() != thesis {
		return errnie.Err(errnie.Validation, "position Thesis mismatch for "+symbol, nil)
	}

	if thesis.LifecycleState(symbol) != types.LifecycleEvaluated {
		return errnie.Err(errnie.Forbidden, "unevaluated position cannot finalize "+symbol, nil)
	}

	desk.positions.Delete(symbol)
	desk.Publish()

	return nil
}

/*
Slots returns the normal allocation capacity configured for strategy. Reserved
capacity remains unavailable unless strategy explicitly selects that class.
*/
func (desk *Desk) Slots() int {
	return desk.maxPositions
}

func (desk *Desk) Buy(
	intent strategy.Intent,
	pair kraken.InstrumentPair,
) error {
	decision := intent.Selected()
	symbol := decision.Symbol
	opportunity := decision.AllocationClass == "reserved"

	if desk.status != types.READY {
		return errnie.Error(errnie.Err(
			errnie.Forbidden,
			"desk not ready to buy",
			nil,
		))
	}

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

	if _, exists := desk.positions.Load(symbol); exists {
		return errnie.Error(errnie.Err(
			errnie.Forbidden,
			"position already exists for "+symbol,
			nil,
		))
	}

	quantity, price, err := desk.quantity(symbol, decision.ProposedNotional, pair)

	if err != nil {
		return err
	}

	position := NewPosition(
		desk.api,
		desk.ui,
		desk.price,
		desk.balance,
		&PositionData{
			Symbol:     symbol,
			Qty:        quantity,
			EntryPrice: price,
		},
		intent.Thesis,
		desk.snapshot,
	)
	/*
		Store the position immediately to reserve its slot.

		If Position.Enter fails, Position.Enter should release the slot
		or notify Desk so the reservation can be removed.
	*/
	desk.positions.Store(symbol, position)

	if err := position.Enter(); err != nil {
		desk.positions.Delete(symbol)

		return err
	}

	return nil
}

/*
quantity converts proposed quote capital into a base quantity at the executable
ask, rounds down to Kraken's increment, and enforces both exchange minima.
*/
func (desk *Desk) quantity(
	symbol string,
	notional float64,
	pair kraken.InstrumentPair,
) (decimal.Decimal, decimal.Decimal, error) {
	ticker, err := desk.price.Get(symbol)

	if err != nil || ticker.Ask == nil || ticker.Ask.Sign() <= 0 {
		return decimal.Decimal{}, decimal.Decimal{}, errnie.Error(errnie.Err(
			errnie.NotFound, "executable ask not available for "+symbol, err,
		))
	}

	fee, err := desk.price.FeeFraction(symbol)

	if err != nil {
		return decimal.Decimal{}, decimal.Decimal{}, err
	}

	if pair.QtyIncrement <= 0 {
		return decimal.Decimal{}, decimal.Decimal{}, errnie.Error(errnie.Err(
			errnie.Validation, "quantity increment required for "+symbol, nil,
		))
	}

	unitCost := ticker.Ask.Float64() * (1 + fee.Float64())
	quantity := math.Floor((notional/unitCost)/pair.QtyIncrement) * pair.QtyIncrement

	if quantity < pair.QtyMin {
		return decimal.Decimal{}, decimal.Decimal{}, errnie.Error(errnie.Err(
			errnie.Forbidden, "proposed quantity below exchange minimum for "+symbol, nil,
		))
	}

	price := *ticker.Ask
	amount := decimal.NewFromFloat64(quantity).SetScale(int64(pair.QtyPrecision))
	orderNotional := desk.price.Notional(price, *amount)

	if pair.CostMin.Float64() > 0 && orderNotional.Cmp(&pair.CostMin) < 0 {
		return decimal.Decimal{}, decimal.Decimal{}, errnie.Error(errnie.Err(
			errnie.Forbidden, "proposed notional below exchange minimum for "+symbol, nil,
		))
	}

	return *amount, price, nil
}

/*
Execute validates and submits the exact action selected by strategy without
reinterpreting it as another portfolio action.
*/
func (desk *Desk) Execute(
	intent strategy.Intent,
	pair kraken.InstrumentPair,
) error {
	decision := intent.Selected()
	intent.Thesis.RecordTrade(types.TradeObservation{
		Kind: "intent_submission", Action: decision.Action,
		Symbol: decision.Symbol, Decision: intent.Decision, At: time.Now(),
	})

	var err error

	switch decision.Action {
	case "enter":
		err = desk.Buy(intent, pair)
	case "exit":
		err = desk.Sell(intent)
	case "reduce":
		err = desk.Sell(intent)
	default:
		err = errnie.Error(errnie.Err(
			errnie.Validation,
			"broker cannot execute action "+decision.Action,
			nil,
		))
	}

	status := "accepted"
	observation := types.TradeObservation{
		Kind: "broker_acceptance", Action: decision.Action,
		Symbol: decision.Symbol, Decision: intent.Decision, Status: status, At: time.Now(),
	}

	if err != nil {
		observation.Kind = "broker_rejection"
		observation.Status = "rejected"
		observation.Error = err.Error()

		next := types.LifecycleRejected

		if decision.Action == "exit" || decision.Action == "reduce" {
			next = types.LifecycleManaging
		}

		if transitionErr := intent.Thesis.Transition(
			decision.Symbol, next, time.Now(),
		); transitionErr != nil {
			errnie.Error(transitionErr)
		}
	}

	intent.Thesis.RecordTrade(observation)

	return err
}

/*
Sell submits the selected exit through the Position already associated with
the Intent's Thesis, preserving one lifecycle across entry and exit.
*/
func (desk *Desk) Sell(intent strategy.Intent) error {
	decision := intent.Selected()
	symbol := decision.Symbol
	position, ok := desk.positions.Load(symbol)

	if !ok || position == nil {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"position not found for "+symbol,
			nil,
		))
	}

	if position.(*Position).Thesis() != intent.Thesis {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"exit Thesis does not own position for "+symbol,
			nil,
		))
	}

	quantity := decimal.NewFromFloat64(decision.ProposedQuantity)

	return position.(*Position).Exit(decision.Action, *quantity)
}

func (desk *Desk) Close() error {
	return nil
}

func (desk *Desk) Executions() []*kraken.Execution {
	executions := make([]*kraken.Execution, 0)

	desk.positions.Range(func(_, value any) bool {
		position := value.(*Position)

		executions = append(
			executions,
			position.Executions()...,
		)

		return true
	})

	return executions
}
