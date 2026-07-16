package broker

import (
	"sync"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

type Desk struct {
	status         types.Status
	api            *websocket.API
	ui             chan []byte
	instrument     *Instrument
	price          *Price
	balance        *Balance
	thesis         *types.Thesis
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
	thesis *types.Thesis,
	messages chan []byte,
) *Desk {
	return &Desk{
		status:       types.INITIALIZING,
		api:          api,
		ui:           messages,
		instrument:   instrument,
		price:        price,
		balance:      balance,
		thesis:       thesis,
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

	for _, holding := range desk.thesis.Positions {
		if holding.Asset == "" || holding.Asset == desk.balance.quote ||
			holding.Qty == nil || holding.Qty.Sign() <= 0 {
			continue
		}

		wallet, err := desk.balance.Holding(holding.Asset)

		if err != nil || wallet.Qty == nil || wallet.Qty.Sign() <= 0 {
			continue
		}

		holding.Qty = wallet.Qty

		if holding.Order != nil {
			holding.Order.Volume = holding.Qty
		}

		pair, err := desk.instrument.Pair(holding.Symbol)

		if err != nil {
			return errnie.Error(err)
		}

		desk.balance.Update(holding.Symbol, holding)

		position := NewPosition(
			desk.api,
			desk.ui,
			desk.instrument,
			desk.price,
			desk.balance,
			&pair,
		)
		position.status = types.OPEN

		desk.positions.Store(holding.Symbol, position)
	}

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

func (desk *Desk) Publish() {
	select {
	case desk.ui <- datura.Map[any]{
		"balances": desk.balance.Snapshot(),
	}.Marshal():
	default:
	}
}

func (desk *Desk) Status() types.Status {
	return desk.status
}

func (desk *Desk) OpenPositions() int {
	count := 0

	desk.positions.Range(func(_, value any) bool {
		position := value.(*Position)

		if position.Stop == nil {
			return true
		}

		if position.Status() == types.PENDING || position.Status() == types.NEW ||
			position.Status() == types.PARTIAL_FILLED {
			count++

			return true
		}

		holding, err := desk.balance.Holding(position.Stop.Symbol)

		if err != nil {
			return true
		}

		if holding.Qty.Sign() > 0 || position.Status() == types.OPEN {
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

		if position.Stop == nil {
			return true
		}

		holding, err := desk.balance.Holding(position.Stop.Symbol)

		if err != nil || holding.Qty.Sign() <= 0 {
			return true
		}

		positions = append(positions, position)

		return true
	})

	return positions
}

func (desk *Desk) Buy(
	order spot.Order,
	pair *kraken.InstrumentPair,
) error {
	if order.Description == nil || order.Volume == nil || order.Volume.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"order description and positive volume required",
			nil,
		))
	}

	symbol := order.Description.Pair
	opportunity := order.Description.OrderType == "reserved"

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

	if existing, exists := desk.positions.Load(symbol); exists {
		status := existing.(*Position).Status()

		if status != types.REJECTED && status != types.CANCELED &&
			status != types.EXPIRED && status != types.ERROR {
			return errnie.Error(errnie.Err(
				errnie.Forbidden,
				"position already exists for "+symbol,
				nil,
			))
		}

		desk.positions.Delete(symbol)
	}

	position := NewPosition(
		desk.api,
		desk.ui,
		desk.instrument,
		desk.price,
		desk.balance,
		pair,
	)

	desk.positions.Store(symbol, position)

	if err := position.Enter(); err != nil {
		desk.positions.Delete(symbol)

		return err
	}

	return nil
}

/*
Sell submits the selected exit through the Position already associated with
the order symbol, preserving one lifecycle across entry and exit.
*/
func (desk *Desk) Sell(order spot.Order) error {
	if order.Description == nil || order.Volume == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"order description and volume required",
			nil,
		))
	}

	symbol := order.Description.Pair
	position, ok := desk.positions.Load(symbol)

	if !ok || position == nil {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"position not found for "+symbol,
			nil,
		))
	}

	return position.(*Position).Exit()
}

func (desk *Desk) Close() error {
	return nil
}
