package broker

import (
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

type SpotHolding struct {
	Asset string
	Qty   decimal.Decimal
}

type Balance struct {
	status    types.Status
	api       *websocket.API
	ui        chan []byte
	model     *kraken.Balance
	quote     string
	observers []func()
}

func NewBalance(api *websocket.API, ui chan []byte) *Balance {
	balance := &Balance{
		status: types.INITIALIZING,
		api:    api,
		ui:     ui,
		quote:  viper.GetString("market.quote_currency"),
	}

	balance.api.On("balances", balance.BalanceAck)
	return balance
}

func (balance *Balance) Status() types.Status {
	return balance.status
}

func (balance *Balance) Publish() {
	if balance.status != types.READY {
		return
	}

	balance.ui <- datura.Map[any]{
		"balances": balance.Snapshot(),
	}.Marshal()
}

/*
Snapshot returns the quote-currency accounting row.
*/
func (balance *Balance) Snapshot() []datura.Map[any] {
	balances := make([]datura.Map[any], 0, 1)

	for _, balanceData := range balance.model.Data {
		if balanceData.Asset != balance.quote {
			continue
		}

		balances = append(balances, datura.Map[any]{
			"asset":     balanceData.Asset,
			"balance":   balanceData.Balance.Float64(),
			"available": balanceData.Available.Float64(),
			"reserved":  balanceData.Reserved.Float64(),
		})
	}

	return balances
}

func (balance *Balance) Initialize() *Balance {
	if errnie.Error(balance.api.SubscribeBalance()) != nil {
		balance.status = types.ERROR
		return balance
	}

	return balance
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

/*
Holdings returns non-quote spot wallet balances that represent open inventory.
*/
func (balance *Balance) Holdings(quote string) []SpotHolding {
	if balance.model == nil {
		return nil
	}

	holdings := make([]SpotHolding, 0)

	for _, balanceData := range balance.model.Data {
		if balanceData.Asset == quote {
			continue
		}

		if strings.Contains(balanceData.Asset, ".") {
			continue
		}

		if balanceData.WalletType != "" && balanceData.WalletType != "spot" {
			continue
		}

		if balanceData.Balance.Sign() <= 0 {
			continue
		}

		holdings = append(holdings, SpotHolding{
			Asset: balanceData.Asset,
			Qty:   balanceData.Balance,
		})
	}

	return holdings
}

func (balance *Balance) Available(amount decimal.Decimal) bool {
	for _, balanceData := range balance.model.Data {
		if balanceData.Asset == balance.quote {
			return balanceData.Available.Sub(&amount).Sign() >= 0
		}
	}

	return false
}
