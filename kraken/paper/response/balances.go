package response

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
)

// ErrInsufficientFunds rejects a fill that the wallet cannot fund in the spent currency.
var ErrInsufficientFunds = errors.New("paper balances: insufficient funds")

/*
Balances simulates the Kraken balances channel on the shared raw bus.

mu guards model/costBasis/realized: fills arrive both from the paper matching
tick (resting triggers, pending takers) and from the quote cache's trade-listener
goroutine (maker queue fills), so wallet state is mutated from two goroutines.
*/
type Balances struct {
	ctx       context.Context
	cancel    context.CancelFunc
	err       error
	pool      *qpool.Q[any]
	isActive  atomic.Bool
	observers []types.Socket
	model     user.Balances
	realized  *big.Rat // running net realized P&L over the session
	holdings  map[string]*big.Rat
	costBasis map[string]*big.Rat // fee-inclusive average cost per base asset
}

func NewBalances(ctx context.Context, pool *qpool.Q[any]) *Balances {
	ctx, cancel := context.WithCancel(ctx)

	return &Balances{
		ctx:       ctx,
		cancel:    cancel,
		err:       nil,
		pool:      pool,
		observers: make([]types.Socket, 0),
		model: user.Balances{
			Asset: []user.Balance{
				{
					Asset:      viper.GetViper().GetString("market.quote_currency"),
					AssetClass: "currency",
					Balance: viper.GetFloat64(
						"trading.paper.wallet." + strings.ToLower(
							viper.GetViper().GetString("market.quote_currency"),
						),
					),
					Wallets: []user.BalanceWallet{{
						Balance: viper.GetFloat64(
							"trading.paper.wallet." + strings.ToLower(
								viper.GetViper().GetString("market.quote_currency"),
							),
						),
						Type: "spot",
						ID:   "main",
					}},
				},
			},
		},
		realized: new(big.Rat),
	}
}

func (balances *Balances) Send(message *qpool.QValue[any]) *types.SocketMessage {
	frame, ok := message.Value.(types.KrakenMessage)

	if !ok {
		return nil
	}

	var out *types.SocketMessage

	switch frame.Method {
	case "subscribe":
		balances.isActive.Store(true)
	case "unsubscribe":
		balances.isActive.Store(false)
		out = &types.SocketMessage{
			Channel: "balances",
			Success: &[]bool{true}[0],
		}
	default:
		return nil
	}

	data, err := sonic.Marshal(balances.model)

	if err != nil {
		return nil
	}

	out = &types.SocketMessage{
		Channel: "balances",
		Success: &[]bool{true}[0],
		Data:    data,
	}

	for _, observer := range balances.observers {
		observer.Send(&qpool.QValue[any]{Value: out})
	}

	return out
}

func (balances *Balances) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		balances.observers = append(balances.observers, socket)
	}
}
