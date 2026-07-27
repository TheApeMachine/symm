package broker

import (
	"sync/atomic"

	"github.com/theapemachine/datura"
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
	status atomic.Value
	ui     chan []byte
}

/*
NewBalance constructs an empty wallet owner. Seed holdings are copied by value
into Inventory so restart inventory is present before the first snapshot.
*/
func NewBalance(
	api *websocket.API,
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
		ui:        ui,
	}

	balance.status.Store(types.INITIALIZING)
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
Publish enqueues Frame on the UI channel. A saturated channel returns an error
instead of dropping the wallet frame silently.
*/
func (balance *Balance) Publish() error {
	wallet := balance.Frame()

	select {
	case balance.ui <- datura.Map[any]{
		"balances": wallet,
	}.Marshal():
		return nil
	default:
		return errnie.Error(errnie.Err(
			errnie.TooManyRequests,
			"balance: ui channel saturated; dropped wallet frame",
			nil,
		))
	}
}

/*
Frame returns wallet rows with available and reserved capital explicit for UI.
Kraken snapshots carry total balance; the broker ledger owns live reservations,
so the terminal must receive both rather than reconstructing unavailable cash.
*/
func (balance *Balance) Frame() map[string]*kraken.BalanceData {
	balance.Wallet.mu.RLock()
	defer balance.Wallet.mu.RUnlock()

	rows := make(map[string]*kraken.BalanceData, len(balance.Wallet.Data))

	for asset, row := range balance.Wallet.Data {
		copyRow := balance.Wallet.clone(row)
		reserved := balance.ReservedAsset(asset)

		if asset == balance.Wallet.quote {
			reserved = balance.ReservedCash()
		}

		copyRow.Reserved = reserved

		if copyRow.Available == nil && copyRow.Balance != nil {
			copyRow.Available = copyRow.Balance.Copy().Sub(reserved)
		}

		rows[asset] = copyRow
	}

	return rows
}

/*
BalanceAck applies one private balance frame. Snapshots replace the wallet map
and sequence; updates require exact next-sequence or a resync is requested.
*/
func (balance *Balance) BalanceAck(buf []byte) {
	ready, resync := balance.Wallet.BalanceAck(buf, func(
		quote string,
		data map[string]*kraken.BalanceData,
		complete bool,
	) {
		balance.Sync(quote, data, complete, balance.ReservedAsset)
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

/*
Holdings returns the live inventory map so UI publishing can marshal the same
shape Balance owns without repackaging restart state for the browser.
*/
func (balance *Balance) Holdings() map[string]*types.Holding {
	return balance.Inventory.Holdings
}

/*
Holding returns one inventory lot by symbol for broker and recovery tests that
need to prove restart-seeded positions are present before exchange snapshots.
*/
func (balance *Balance) Holding(symbol string) (*types.Holding, error) {
	holding, ok := balance.Inventory.Holdings[symbol]

	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"balance: holding not found",
			nil,
		))
	}

	return holding, nil
}
