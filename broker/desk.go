package broker

import (
	"strings"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
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

	for _, trade := range history.Result.Trades {
		fixed := strings.ReplaceAll(
			trade.Pair,
			desk.balance.quote,
			"",
		) + "/" + desk.balance.quote

		/*
			Ask Price to calculate the complete current position value.

			Desk does not calculate:
			  - notionals
			  - fees
			  - PnL
			  - return percentage
		*/
		quote, err := desk.price.PositionQuote(
			fixed,
			*trade.Price,
			*trade.Volume,
		)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to quote position "+fixed,
				err,
			))

			continue
		}

		position := NewPosition(
			desk.api,
			desk.ui,
			desk.price,
			desk.balance,
			&PositionData{
				Symbol:     fixed,
				Qty:        *trade.Volume,
				EntryPrice: *trade.Price,
				Mark:       quote.Mark,
				PnL:        quote.PnL,
				ReturnPct:  quote.ReturnPct,
			},
		).Hydrate(fixed, history)

		desk.positions.Store(fixed, position)
	}

	desk.status = types.READY
	desk.Publish()

	return nil
}

func (desk *Desk) Publish() {
	desk.positions.Range(func(_, value any) bool {
		position := value.(*Position)
		position.Publish()
		return true
	})

	desk.balance.Publish()
	desk.price.Publish()
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
	positions := make([]*Position, 0)

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

	position := NewPosition(
		desk.api,
		desk.ui,
		desk.price,
		desk.balance,
		&PositionData{
			Symbol:     symbol,
			Qty:        *decimal.NewFromFloat64(fraction),
			EntryPrice: price,
		},
	)

	/*
		Store the position immediately to reserve its slot.

		If Position.Enter fails, Position.Enter should release the slot
		or notify Desk so the reservation can be removed.
	*/
	desk.positions.Store(symbol, position)

	return position.Enter()
}

func (desk *Desk) Sell(symbol string) error {
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
