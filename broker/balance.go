package broker

import (
	"context"
	"github.com/theapemachine/symm/nomagique/runtime"
	"sort"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/websocket"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
Balance owns the exchange wallet map for the Desk event loop.

The wallet is exchange-owned in the strictest sense: the REST balance endpoint
is its only writer. Nothing here adds, subtracts, or reserves against it, so
what the desk reads is always what the venue last stated it holds rather than a
locally maintained approximation of it. Capital committed by an entry becomes
visible the same way everything else does — on the next refresh.
*/
type Balance struct {
	status types.Status
	api    *websocket.API
	ui     *runtime.Channel[*types.UIFrame]
	wallet *sync.Map
	quote  string
}

/*
NewBalance constructs an empty wallet owner for exchange balances only.
*/
func NewBalance(
	ctx context.Context,
	bus *runtime.Workspace,
) *Balance {
	if bus == nil {
		panic("broker: workspace bus required")
	}

	var api *websocket.API
	if shared, _ := bus.Shared("api", ""); shared != nil {
		api, _ = shared.(*websocket.API)
	}
	if api == nil {
		panic("broker: api not found in workspace")
	}

	ui := runtime.ChannelOf(bus, types.ChannelUI, func(frame *types.UIFrame) string { return "" })

	balance := &Balance{
		status: types.READY,
		api:    api,
		ui:     ui,
		wallet: &sync.Map{},
		quote:  viper.GetViper().GetString("market.quote_currency"),
	}

	balance.Update()
	return balance
}

/*
Status reports wallet readiness for Desk and stack health checks.
*/
func (balance *Balance) Status() types.Status {
	return balance.status
}

func (balance *Balance) Wallet() *types.UIFrame {
	balances := make([]*wire.BalanceT, 0)

	balance.wallet.Range(func(key, value any) bool {
		asset, ok := key.(string)

		if !ok {
			return true
		}

		amount, ok := value.(*decimal.Decimal)

		if !ok {
			return true
		}

		balances = append(balances, &wire.BalanceT{
			Asset:  asset,
			Amount: amount.String(),
		})

		return true
	})

	sort.Slice(balances, func(left, right int) bool {
		return balances[left].Asset < balances[right].Asset
	})

	return &wire.FrameT{
		Type: wire.FrameBalancesFrame,
		Value: &wire.BalancesFrameT{
			Balances: balances,
		},
	}
}

func (balance *Balance) Update() {
	result, err := balance.api.Balance()

	if err != nil {
		balance.status = types.ERROR
		return
	}

	balance.replace(result)
	balance.status = types.READY
}

/*
replace swaps the wallet contents for a complete REST balance response.

The endpoint reports the whole wallet, omitting assets the account no longer
holds, so the map is replaced rather than merged. Merging would leave a
sold-out asset sitting at its last positive quantity, and since a per-coin
balance is what constitutes a position here, that stale row is indistinguishable
from an open one.
*/
func (balance *Balance) replace(totals map[string]*decimal.Decimal) {
	balance.wallet.Range(func(key, _ any) bool {
		asset, ok := key.(string)

		if !ok {
			return true
		}

		if _, held := totals[asset]; !held {
			balance.wallet.Delete(key)
		}

		return true
	})

	for asset, amount := range totals {
		balance.wallet.Store(asset, amount)
	}
}

/*
Assets exposes the wallet as a plain per-coin map.

Everything the account holds other than the quote currency is a position, so
callers reconciling lots against the venue read the wallet directly rather than
inferring inventory from anywhere else.
*/
func (balance *Balance) Assets() map[string]*decimal.Decimal {
	assets := map[string]*decimal.Decimal{}

	balance.wallet.Range(func(key, value any) bool {
		asset, ok := key.(string)

		if !ok {
			return true
		}

		amount, ok := value.(*decimal.Decimal)

		if !ok || amount == nil {
			return true
		}

		assets[asset] = amount
		return true
	})

	return assets
}

/*
Cash reports the quote balance exactly as the exchange last stated it.
*/
func (balance *Balance) Cash() *decimal.Decimal {
	found, ok := balance.wallet.Load(balance.quote)

	if !ok || found == nil {
		return nil
	}

	cash, ok := found.(*decimal.Decimal)

	if !ok {
		return nil
	}

	return cash
}
