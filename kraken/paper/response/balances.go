package response

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
)

/*
Balances simulates the Kraken balances channel on the shared raw bus.

Wallet state is immutable per snapshot and swapped with atomic.Pointer so
concurrent fill and mark paths never use mutexes or channels.
*/
type Balances struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	pool          *qpool.Q[any]
	isActive      atomic.Bool
	observers     []types.Socket
	quoteCurrency string
	model         user.Balances
}

func NewBalances(
	ctx context.Context, pool *qpool.Q[any],
) *Balances {
	ctx, cancel := context.WithCancel(ctx)
	quote := strings.ToUpper(viper.GetString("market.quote_currency"))

	return &Balances{
		ctx:           ctx,
		cancel:        cancel,
		pool:          pool,
		observers:     make([]types.Socket, 0),
		quoteCurrency: quote,
		model: user.Balances{
			Asset: []user.Balance{{
				Asset:      viper.GetString("market.quote_currency"),
				AssetClass: "currency",
				Balance: viper.GetFloat64(
					"trading.paper.wallet." + strings.ToLower(quote),
				),
				Wallets: []user.BalanceWallet{{
					Balance: viper.GetFloat64(
						"trading.paper.wallet." + strings.ToLower(quote),
					),
					Type: "spot",
					ID:   "main",
				}},
			}},
		},
	}
}

func (balances *Balances) Send(message []byte) *types.SocketMessage {
	var request types.KrakenMessage

	if err := sonic.Unmarshal(message, &request); err != nil {
		return nil
	}

	switch request.Method {
	case "subscribe":
		balances.isActive.Store(true)
	case "unsubscribe":
		balances.isActive.Store(false)
	default:
		return nil
	}

	return balances.snapshotMessage(user.BalanceSnapshot)
}

func (balances *Balances) snapshotMessage(messageType string) *types.SocketMessage {
	data, err := sonic.Marshal(balances.model)

	if err != nil {
		return nil
	}

	for _, socket := range balances.observers {
		socket.Send(data)
	}

	return &types.SocketMessage{
		Channel: "balances",
		Type:    messageType,
		Success: true,
		Data:    data,
	}
}

func (balances *Balances) PublishUpdate() {
	if !balances.isActive.Load() || balances.pool == nil {
		return
	}

	balances.routeSocketMessage(user.BalanceUpdate)
}

func (balances *Balances) routeSocketMessage(messageType string) {
	message := balances.snapshotMessage(messageType)

	if message == nil {
		return
	}

	buffer, err := sonic.Marshal(message)

	if err != nil {
		return
	}

	out := datura.Acquire("kraken:private", datura.Artifact_Type_json).
		WithDestination("kraken:socket").
		WithRole(message.Channel).
		WithScope(message.Type).
		WithPayload(buffer)

	errnie.Error(
		balances.pool.CreateBroadcastGroup("kraken:socket").Send(out),
	)
}

func (balances *Balances) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		balances.observers = append(balances.observers, socket)
	}
}

func (balances *Balances) ApplyFill(notice FillNotice) {
	base, quote := symbolParts(notice.Params.Symbol)

	if base == "" || quote == "" || notice.Price <= 0 || notice.Params.OrderQty <= 0 {
		return
	}

	cost := notice.Price * notice.Params.OrderQty

	switch notice.Params.Side {
	case trading.Buy:
		balances.adjustAsset(quote, -cost)
		balances.adjustAsset(base, notice.Params.OrderQty)
	case trading.Sell:
		balances.adjustAsset(base, -notice.Params.OrderQty)
		balances.adjustAsset(quote, cost)
	}
}

func (balances *Balances) adjustAsset(asset string, delta float64) {
	if asset == "" || delta == 0 {
		return
	}

	for index, row := range balances.model.Asset {
		if row.Asset != asset {
			continue
		}

		row.Balance += delta
		balances.model.Asset[index] = row

		if len(row.Wallets) > 0 {
			row.Wallets[0].Balance += delta
			balances.model.Asset[index].Wallets = row.Wallets
		}

		return
	}

	balances.model.Asset = append(balances.model.Asset, user.Balance{
		Asset:      asset,
		AssetClass: "currency",
		Balance:    delta,
		Wallets: []user.BalanceWallet{{
			Balance: delta,
			Type:    "spot",
			ID:      "main",
		}},
	})
}

func (balances *Balances) Clone() *Balances {
	return &Balances{
		ctx:           balances.ctx,
		cancel:        balances.cancel,
		pool:          balances.pool,
		observers:     balances.observers,
		quoteCurrency: balances.quoteCurrency,
		model:         balances.model,
	}
}
