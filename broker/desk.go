package broker

import (
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

type Desk struct {
	status       types.Status
	api          *websocket.API
	ui           chan []byte
	price        *Price
	balance      *Balance
	positions    *sync.Map
	maxPositions int
	maxReserved  int
}

func NewDesk(
	api *websocket.API,
	price *Price,
	balance *Balance,
	messages chan []byte,
) *Desk {
	desk := &Desk{
		status:       types.INITIALIZING,
		api:          api,
		ui:           messages,
		price:        price,
		balance:      balance,
		positions:    &sync.Map{},
		maxPositions: viper.GetViper().GetInt("trading.slots.normal"),
		maxReserved:  viper.GetViper().GetInt("trading.slots.reserved"),
	}

	return desk
}

func (desk *Desk) Status() types.Status {
	return desk.status
}

func (desk *Desk) OpenPositions() int {
	count := 0

	desk.positions.Range(func(_, _ any) bool {
		count++
		return true
	})

	return count
}

func (desk *Desk) Positions() []*Position {
	positions := make([]*Position, 0, desk.OpenPositions())

	desk.positions.Range(func(_, value any) bool {
		positions = append(positions, value.(*Position))
		return true
	})

	return positions
}

func (desk *Desk) Buy(
	symbol string,
	fraction float64,
	price decimal.Decimal,
	opportunity bool,
) error {
	if desk.status != types.READY {
		return errnie.Error(errnie.Err(
			errnie.Forbidden, "desk not ready to buy", nil,
		))
	}

	openPositions := desk.OpenPositions()

	if openPositions >= desk.maxPositions+desk.maxReserved {
		return errnie.Error(errnie.Err(
			errnie.Forbidden, "desk at max positions and reserved", nil,
		))
	}

	if openPositions >= desk.maxPositions && !opportunity {
		return errnie.Error(errnie.Err(
			errnie.Forbidden, "desk at max positions", nil,
		))
	}

	position := NewPosition(
		desk.api,
		desk.ui,
		desk.price,
		desk.balance,
		&PositionData{
			Symbol:     symbol,
			Qty:        fraction,
			EntryPrice: price,
		},
	)

	// Store the position for now, so we make a claim on the slot. In case
	// of any errors, or other issues, we can release the slot. This is not
	// a guarantee that the position will be opened, it just reserves the
	// slot, so we don't end up with multiple positions racing to open.
	desk.positions.Store(symbol, position)
	return position.Enter()
}

func (desk *Desk) Sell(symbol string) error {
	position, ok := desk.positions.Load(symbol)

	if !ok {
		return errnie.Error(errnie.Err(
			errnie.NotFound, "position not found for "+symbol, nil,
		))
	}

	if position == nil {
		return errnie.Error(errnie.Err(
			errnie.NotFound, "position not found for "+symbol, nil,
		))
	}

	return position.(*Position).Exit()
}

func (desk *Desk) Close() error {
	return nil
}
