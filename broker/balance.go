package broker

import (
	"maps"
	"sort"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/* balanceSnapshot is one complete immutable REST balance observation. */
type balanceSnapshot struct {
	assets map[string]*decimal.Decimal
	from   time.Time
	status types.Status
}

/*
	Balance publishes complete exchange-owned wallet maps atomically. Reservations

belong to execution admission and never modify these authoritative balances.
*/
type Balance struct {
	api      *websocket.API
	quote    string
	snapshot atomic.Pointer[balanceSnapshot]
}

/* NewBalance binds the existing account and fetches its initial holdings. */
func NewBalance(api *websocket.API) *Balance {
	if api == nil {
		panic("broker: api required")
	}
	balance := &Balance{api: api, quote: viper.GetViper().GetString("market.quote_currency")}
	balance.Update()
	return balance
}

/* Status reports the latest REST observation's readiness. */
func (balance *Balance) Status() types.Status { return balance.snapshot.Load().status }

/* Wallet publishes the same authoritative assets used by execution and recovery. */
func (balance *Balance) Wallet() *types.UIFrame {
	snapshot := balance.snapshot.Load()
	balances := make([]*wire.BalanceT, 0, len(snapshot.assets))
	for asset, amount := range snapshot.assets {
		balances = append(balances, &wire.BalanceT{Asset: asset, Amount: amount.String()})
	}
	sort.Slice(balances, func(left, right int) bool { return balances[left].Asset < balances[right].Asset })
	return &wire.FrameT{Type: wire.FrameBalancesFrame, Value: &wire.BalancesFrameT{Balances: balances}}
}

/* Update replaces the entire map; readers cannot observe a partially refreshed account. */
func (balance *Balance) Update() {
	from := time.Now().UTC()
	result, err := balance.api.Balance()

	if err != nil {
		failed := balanceSnapshot{status: types.ERROR, from: from}

		if previous := balance.snapshot.Load(); previous != nil {
			failed.assets = previous.assets
		}
		balance.snapshot.Store(&failed)
		errnie.Error(err)
		return
	}
	canonical := make(map[string]*decimal.Decimal, len(result))
	for asset, amount := range result {
		asset = balance.api.Normalizer().Name(asset)

		if _, exists := canonical[asset]; exists {
			failed := balanceSnapshot{status: types.ERROR, from: from}

			if previous := balance.snapshot.Load(); previous != nil {
				failed.assets = previous.assets
			}
			balance.snapshot.Store(&failed)
			errnie.Error(errnie.Err(errnie.Conflict, "balance: multiple venue assets normalize to "+asset, nil))
			return
		}
		canonical[asset] = amount
	}
	balance.snapshot.Store(&balanceSnapshot{assets: canonical, from: from, status: types.READY})
}

/* Assets copies the current account map for callers that retain or transform it. */
func (balance *Balance) Assets() map[string]*decimal.Decimal {
	return maps.Clone(balance.snapshot.Load().assets)
}

/* Cash reports the quote balance exactly as the exchange last stated it. */
func (balance *Balance) Cash() *decimal.Decimal { return balance.snapshot.Load().assets[balance.quote] }
