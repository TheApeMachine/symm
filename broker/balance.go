package broker

import (
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Balance owns the exchange wallet map and sequence for the Desk event loop. It
composes Wallet, Cash, and Ledger so reservations and available capital stay a
wallet concern while Position and Holding ownership remains on Desk.
*/
type Balance struct {
	status types.Status
	api    *websocket.API
	ui     chan []byte
	wallet *sync.Map
	quote  string
}

/*
NewBalance constructs an empty wallet owner for exchange balances only.
*/
func NewBalance(
	api *websocket.API,
	ui chan []byte,
	market config.MarketConfig,
) *Balance {
	balance := &Balance{
		status: types.INITIALIZING,
		api:    api,
		ui:     ui,
		wallet: &sync.Map{},
		quote:  viper.GetViper().GetString("market.quote_currency"),
	}

	return balance
}

/*
Status reports wallet readiness for Desk and stack health checks.
*/
func (balance *Balance) Status() types.Status {
	return balance.status
}

func (balance *Balance) Update() {
	result, err := balance.api.Balance()

	if err != nil {
		balance.status = types.ERROR
		return
	}

	for asset, amount := range result {
		balance.wallet.Store(asset, amount)
	}

	balance.status = types.READY
}

func (balance *Balance) Cash() (*decimal.Decimal, error) {
	found, ok := balance.wallet.Load(balance.quote)

	if !ok || found == nil {
		return nil, nil
	}

	cash, ok := found.(*decimal.Decimal)

	if !ok {
		return nil, nil
	}

	return cash, nil
}

func (balance *Balance) Reserve(amount *decimal.Decimal) error {
	if amount == nil || amount.Sign() <= 0 {
		return nil
	}

	cash, err := balance.Cash()
	if err != nil || cash == nil || cash.Sign() <= 0 {
		return err
	}

	if cash.Cmp(amount) < 0 {
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"insufficient cash to reserve",
			nil,
		))
	}

	newCash := cash.Sub(amount)
	balance.wallet.Store(balance.quote, newCash)

	return nil
}

func (balance *Balance) Release(amount *decimal.Decimal) error {
	if amount == nil || amount.Sign() <= 0 {
		return nil
	}

	cash, err := balance.Cash()
	if err != nil || cash == nil || cash.Sign() < 0 {
		return err
	}

	newCash := cash.Add(amount)
	balance.wallet.Store(balance.quote, newCash)

	return nil
}
