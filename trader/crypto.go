package trader

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
Crypto listens to actions and routes them through the broker desk.
*/
type Crypto struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	desk        *broker.Desk
	balance     *user.Balance
}

func NewCrypto(ctx context.Context, pool *qpool.Q) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		desk: errnie.Does(func() (*broker.Desk, error) {
			return broker.NewDesk(ctx, pool)
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value(),
		balance: errnie.Does(func() (*user.Balance, error) {
			return user.NewBalance(ctx)
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value(),
	}

	for _, channel := range []string{"actions"} {
		crypto.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		crypto.subscribers[channel] = crypto.broadcasts[channel].Subscribe("trader:"+channel, 128)
	}

	return crypto, errnie.Error(errnie.Require(map[string]any{
		"ctx":     ctx,
		"cancel":  cancel,
		"pool":    pool,
		"desk":    crypto.desk,
		"balance": crypto.balance,
	}))
}

func (crypto *Crypto) Tick() error {
	var (
		action perspectives.Action
		ok     bool
	)

	for row := range crypto.subscribers["actions"].Incoming {
		if row == nil {
			errnie.Warn("nil action")
			continue
		}

		if action, ok = row.Value.(perspectives.Action); !ok {
			errnie.Warn("invalid action")
			continue
		}

		switch action.Type {
		case perspectives.ActionEnter:
			errnie.Error(crypto.desk.AddOrder(
				action.Symbol, trading.Buy, action.Price, action.Quantity,
			))
		case perspectives.ActionExit:
			errnie.Error(crypto.desk.AddOrder(
				action.Symbol, trading.Sell, action.Price, action.Quantity,
			))
		case perspectives.ActionStopLoss:
			errnie.Error(crypto.desk.AddOrder(
				action.Symbol, trading.Sell, action.Price, action.Quantity,
			))
		case perspectives.ActionTakeProfit:
			errnie.Error(crypto.desk.AddOrder(
				action.Symbol, trading.Buy, action.Price, action.Quantity,
			))
		case perspectives.ActionShort:
			errnie.Error(crypto.desk.AddOrder(
				action.Symbol, trading.Sell, action.Price, action.Quantity,
			))
		}
	}

	return crypto.err
}

func (crypto *Crypto) Close() error {
	crypto.cancel()
	return nil
}
