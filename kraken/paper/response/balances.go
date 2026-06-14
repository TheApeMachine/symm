package response

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
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
	var out *types.SocketMessage

	if err := sonic.Unmarshal(message, &out); err != nil {
		return nil
	}

	switch out.Method {
	case "subscribe":
		balances.isActive.Store(true)
	case "unsubscribe":
		balances.isActive.Store(false)
	default:
		return nil
	}

	data, err := sonic.Marshal(balances.model)

	if err != nil {
		return nil
	}

	out = &types.SocketMessage{
		Channel: "balances",
		Success: true,
		Data:    data,
	}

	for _, socket := range balances.observers {
		socket.Send(data)
	}

	return out
}

func (balances *Balances) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		balances.observers = append(balances.observers, socket)
	}
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
