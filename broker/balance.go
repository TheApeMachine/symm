package broker

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

type Balance struct {
	status types.Status
	api    *websocket.API
	ui     chan []byte
	model  *kraken.Balance
	quote  string
}

func NewBalance(api *websocket.API, ui chan []byte) *Balance {
	balance := &Balance{
		status: types.INITIALIZING,
		api:    api,
		ui:     ui,
		quote:  viper.GetViper().GetString("trading.market_quote"),
	}

	balance.api.On("balances", balance.BalanceAck)
	return balance
}

func (balance *Balance) Status() types.Status {
	return balance.status
}

func (balance *Balance) Publish() {
	select {
	case balance.ui <- datura.Map[any]{
		"balance": balance.model,
	}.Marshal():
	default:
	}
}

func (balance *Balance) Initialize() error {
	return balance.api.SubscribeBalance()
}

func (balance *Balance) BalanceAck(buf []byte) {
	balance.model = kraken.NewBalance(buf)

	if errnie.Error(kraken.Validate(balance.model)) != nil {
		return
	}

	balance.status = types.READY
	balance.Publish()
}

func (balance *Balance) Get(symbol string) (*kraken.BalanceData, error) {
	for _, balanceData := range balance.model.Data {
		if balanceData.Asset == symbol {
			return &balanceData, nil
		}
	}

	return nil, errnie.Error(errnie.Err(
		errnie.NotFound,
		"balance not found for "+symbol,
		nil,
	))
}

func (balance *Balance) Available(amount decimal.Decimal) bool {
	for _, balanceData := range balance.model.Data {
		if balanceData.Asset == balance.quote {
			if balanceData.Amount.Sub(&amount).Int64() > 0 {
				return true
			}
		}
	}

	return false
}
