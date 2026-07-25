package broker

import (
	"sync"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Balance owns the exchange wallet map and sequence for the Desk event loop.
It embeds Ledger and Inventory so reservations and lots stay separate owners;
cash availability and UI projection are Balance methods over those parts.
*/
type Balance struct {
	*Ledger
	*Inventory
	status   atomic.Value
	api      *websocket.API
	mu       sync.RWMutex
	data     map[string]*kraken.BalanceData
	quote    string
	ui       chan []byte
	sequence int64
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
	balance := &Balance{
		Ledger:    NewLedger(),
		Inventory: NewInventory(),
		api:       api,
		quote:     market.QuoteCurrency,
		data:      make(map[string]*kraken.BalanceData),
		ui:        ui,
		sequence:  0,
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

	if balance.api == nil {
		balance.status.Store(types.READY)
		return nil
	}

	balance.status.Store(types.PENDING)

	if errnie.Error(balance.api.SubscribeBalance()) != nil {
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
	incoming := kraken.NewBalance(buf)

	if errnie.Error(kraken.Validate(incoming)) != nil {
		return
	}

	balance.mu.Lock()

	if incoming.Type == "snapshot" {
		replaced := make(map[string]*kraken.BalanceData, len(incoming.Data))

		for index := range incoming.Data {
			row := incoming.Data[index]
			replaced[row.Asset] = balance.clone(&row)
		}

		balance.data = replaced
		balance.sequence = incoming.Sequence
		balance.Sync(balance.quote, balance.data, balance.ReservedAsset)
		balance.mu.Unlock()
		balance.status.Store(types.READY)

		if err := balance.Publish(); err != nil {
			errnie.Error(err)
		}

		return
	}

	if balance.sequence > 0 && incoming.Sequence != balance.sequence+1 {
		balance.mu.Unlock()
		errnie.Error(errnie.Err(
			errnie.Validation,
			"balance: sequence gap requires snapshot resync",
			nil,
		))
		balance.resync()

		return
	}

	for index := range incoming.Data {
		row := incoming.Data[index]
		balance.data[row.Asset] = balance.clone(&row)
	}

	balance.sequence = incoming.Sequence
	balance.Sync(balance.quote, balance.data, balance.ReservedAsset)
	balance.mu.Unlock()
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
	if balance.api == nil {
		balance.status.Store(types.ERROR)

		return
	}

	if errnie.Error(balance.api.SubscribeBalance()) != nil {
		balance.status.Store(types.ERROR)
	}
}

/*
clone deep-copies one wallet row so callers and the stored map never share
mutable decimal state.
*/
func (balance *Balance) clone(row *kraken.BalanceData) *kraken.BalanceData {
	cloned := *row

	if row.Balance != nil {
		cloned.Balance = row.Balance.Copy()
	}

	return &cloned
}

/*
Get returns a copied wallet row for symbol so readers cannot mutate the map.
*/
func (balance *Balance) Get(symbol string) (*kraken.BalanceData, error) {
	if err := errnie.Require(map[string]any{
		"symbol": symbol,
		"data":   balance.data,
	}); err != nil {
		return nil, errnie.Error(err)
	}

	balance.mu.RLock()
	defer balance.mu.RUnlock()

	row, ok := balance.data[symbol]

	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"balance not found for "+symbol,
			nil,
		))
	}

	return balance.clone(row), nil
}

/*
Available reports whether free quote cash covers amount after live reservations.
*/
func (balance *Balance) Available(amount *decimal.Decimal) (bool, error) {
	if err := errnie.Require(map[string]any{
		"amount": amount,
		"data":   balance.data,
	}); err != nil {
		return false, errnie.Error(err)
	}

	free, err := balance.FreeCash()

	if err != nil {
		return false, err
	}

	return free.Sub(amount).Sign() >= 0, nil
}

/*
FreeCash is exchange quote balance minus open cash reservations on the ledger.
*/
func (balance *Balance) FreeCash() (*decimal.Decimal, error) {
	row, err := balance.Get(balance.quote)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"quote balance not found",
			err,
		))
	}

	if row.Balance == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"quote balance not found",
			nil,
		))
	}

	return row.Balance.Copy().Sub(balance.ReservedCash()), nil
}

/*
AssetAvailable returns sellable base qty after subtracting sell reservations.
*/
func (balance *Balance) AssetAvailable(asset string) (*decimal.Decimal, error) {
	if err := errnie.Require(map[string]any{
		"asset": asset,
		"data":  balance.data,
	}); err != nil {
		return nil, errnie.Error(err)
	}

	row, err := balance.Get(asset)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"asset available balance missing for "+asset,
			err,
		))
	}

	if row.Balance == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"asset available balance missing for "+asset,
			nil,
		))
	}

	return row.Balance.Copy().Sub(balance.ReservedAsset(asset)), nil
}
