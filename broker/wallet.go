package broker

import (
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
)

/*
Wallet owns the exchange balance map and sequence baseline for one quote
currency. Snapshot and update frames mutate data under Wallet.mu only.
*/
type Wallet struct {
	mu       sync.RWMutex
	Data     map[string]*kraken.BalanceData
	quote    string
	sequence int64
	api      *websocket.API
}

/*
newWallet constructs an empty wallet map for Balance to compose.
*/
func newWallet(api *websocket.API, quote string) *Wallet {
	return &Wallet{
		api:   api,
		quote: quote,
		Data:  make(map[string]*kraken.BalanceData),
	}
}

/*
Get returns a copied wallet row for symbol so readers cannot mutate the map.
*/
func (wallet *Wallet) Get(symbol string) (*kraken.BalanceData, error) {
	wallet.mu.RLock()
	defer wallet.mu.RUnlock()

	if err := errnie.Require(map[string]any{
		"symbol": symbol,
		"data":   wallet.Data,
	}); err != nil {
		return nil, errnie.Error(err)
	}

	row, ok := wallet.Data[symbol]

	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"balance not found for "+symbol,
			nil,
		))
	}

	return wallet.clone(row), nil
}

/*
BalanceAck applies one private balance frame. Snapshots replace the wallet map
and sequence; updates require exact next-sequence or a resync is requested.
*/
func (wallet *Wallet) BalanceAck(
	buf []byte,
	sync func(quote string, data map[string]*kraken.BalanceData, complete bool),
) (ready bool, resync bool) {
	incoming := kraken.NewBalance(buf)

	if errnie.Error(kraken.Validate(incoming)) != nil {
		return false, false
	}

	wallet.mu.Lock()

	if incoming.Type == "snapshot" {
		replaced := make(map[string]*kraken.BalanceData, len(incoming.Data))

		for index := range incoming.Data {
			row := incoming.Data[index]
			replaced[row.Asset] = wallet.clone(&row)
		}

		wallet.Data = replaced
		wallet.sequence = incoming.Sequence
		sync(wallet.quote, wallet.Data, true)
		wallet.mu.Unlock()

		return true, false
	}

	if wallet.sequence > 0 && incoming.Sequence != wallet.sequence+1 {
		wallet.mu.Unlock()

		errnie.Error(errnie.Err(
			errnie.Validation,
			"balance: sequence gap requires snapshot resync",
			nil,
		))

		return false, true
	}

	for index := range incoming.Data {
		row := incoming.Data[index]
		wallet.Data[row.Asset] = wallet.clone(&row)
	}

	wallet.sequence = incoming.Sequence
	sync(wallet.quote, wallet.Data, false)
	wallet.mu.Unlock()

	return true, false
}

/*
clone deep-copies one wallet row so callers and the stored map never share
mutable decimal state.
*/
func (wallet *Wallet) clone(row *kraken.BalanceData) *kraken.BalanceData {
	cloned := *row

	if row.Balance != nil {
		cloned.Balance = row.Balance.Copy()
	}

	if row.Available != nil {
		cloned.Available = row.Available.Copy()
	}

	if row.Reserved != nil {
		cloned.Reserved = row.Reserved.Copy()
	}

	return &cloned
}
