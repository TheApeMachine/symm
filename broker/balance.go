package broker

import (
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Balance owns the exchange wallet map and sequence for the Desk event loop.
It composes Wallet, Cash, View, Ledger, and Inventory so reservations, lots,
availability, and UI projection stay separate owners.
*/
type Balance struct {
	*Ledger
	*Inventory
	*Wallet
	*Cash
	*View
	status atomic.Value
	ui     chan []byte
}

/*
NewBalance constructs an empty wallet owner. Seed holdings are copied by value
into Inventory so restart inventory is present before the first snapshot.
*/
func NewBalance(
	api *websocket.API,
	holdings []types.Holding,
	ui chan []byte,
	market config.MarketConfig,
) *Balance {
	ledger := NewLedger()
	inventory := NewInventory()
	wallet := newWallet(api, market.QuoteCurrency)

	balance := &Balance{
		Ledger:    ledger,
		Inventory: inventory,
		Wallet:    wallet,
		Cash:      &Cash{wallet: wallet, ledger: ledger},
		View: &View{
			wallet:    wallet,
			ledger:    ledger,
			inventory: inventory,
			ui:        ui,
		},
		ui: ui,
	}
	balance.status.Store(types.INITIALIZING)

	for _, holding := range holdings {
		lot := holding
		balance.StoreHolding(&lot)
	}

	return balance
}

/*
Initialize subscribes the private balances channel when an API is attached.
*/
func (balance *Balance) Initialize() error {
	errnie.Info("initializing balance")

	if balance.Wallet.api == nil {
		balance.status.Store(types.READY)
		return nil
	}

	balance.status.Store(types.PENDING)

	if errnie.Error(balance.Wallet.api.SubscribeBalance()) != nil {
		balance.status.Store(types.ERROR)

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to subscribe to balance",
			nil,
		))
	}

	return nil
}

/*
Status reports wallet readiness for Desk and stack health checks.
*/
func (balance *Balance) Status() types.Status {
	return balance.status.Load().(types.Status)
}

/*
BalanceAck applies one private balance frame. Snapshots replace the wallet map
and sequence; updates require exact next-sequence or a resync is requested.
*/
func (balance *Balance) BalanceAck(buf []byte) {
	ready, resync := balance.Wallet.BalanceAck(buf, func(
		quote string,
		data map[string]*kraken.BalanceData,
	) {
		balance.Sync(quote, data, balance.ReservedAsset)
	})

	if resync {
		balance.resync()

		return
	}

	if !ready {
		return
	}

	balance.status.Store(types.READY)

	if err := balance.Publish(); err != nil {
		errnie.Error(err)
	}
}

/*
resync requests a fresh balances snapshot after a sequence gap so the wallet
does not apply updates against an unknown baseline.
*/
func (balance *Balance) resync() {
	if balance.Wallet.api == nil {
		balance.status.Store(types.ERROR)

		return
	}

	if errnie.Error(balance.Wallet.api.SubscribeBalance()) != nil {
		balance.status.Store(types.ERROR)
	}
}
