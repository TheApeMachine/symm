package broker

import (
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
Initialize reconnects persisted Thesis holdings to authoritative wallet
quantities and creates their Position managers. The booter calls this once.
*/
func (desk *Desk) Initialize() error {
	errnie.Info("initializing desk")

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

func (desk *Desk) OpenPositions() int {
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
) error {
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
		if exists {
			return errnie.Error(errnie.Err(
				errnie.Forbidden,
				"position already exists for "+holding.Symbol,
				nil,
			))
		}
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

	desk.positions.Store(holding.Symbol, position)

	if err := position.Enter(); err != nil {
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
