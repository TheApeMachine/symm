package trader

import (
	"context"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
)

/*
Crypto is a trader that is responsible for orchestrating
the trading of crypto assets. It should collect the data
it needs to make informed decisions regarding the opening,
closing, and reporting of positions.
*/
type Crypto struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q[any]
	broadcasts  *sync.Map
	subscribers *sync.Map
	instrument  *Instrument
	book        *Book
	ticker      *Ticker
	trade       *Trade
	ohlc        *OHLC
	action      *Action
	execution   *Execution
	balances    *Balances
}

func NewCrypto(
	ctx context.Context, pool *qpool.Q[any],
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:        ctx,
		cancel:     cancel,
		pool:       pool,
		instrument: NewInstrument(ctx),
		book:       NewBook(ctx),
		ticker:     NewTicker(ctx),
		trade:      NewTrade(ctx),
		ohlc:       NewOHLC(ctx),
		action:     NewAction(ctx),
		execution:  NewExecution(ctx),
		balances:   NewBalances(ctx),
	}

	for _, channel := range []string{
		"desk", "ui",
	} {
		crypto.broadcasts.Store(
			channel, pool.CreateBroadcastGroup(channel),
		)
	}

	for _, channel := range []string{
		"ticker", "book", "trade", "action", "execution", "balances",
	} {
		crypto.subscribers.Store(
			channel, pool.Subscribe(channel, crypto.onMessage),
		)
	}

	return crypto
}

func (crypto *Crypto) onMessage(artifact *datura.Artifact) error {
	origin := errnie.Does(func() (string, error) {
		return artifact.Origin()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"crypto: failed to get origin",
			err,
		))
	}).Value()

	switch origin {
	case "ticker":
		crypto.ticker.Update(
			datura.As[*market.TickerUpdates](artifact),
		)
	case "book":
		crypto.book.Update(
			datura.As[*market.BookUpdates](artifact),
		)
	case "trade":
		crypto.trade.Update(
			datura.As[*market.TradeUpdates](artifact),
		)
	case "action":
		crypto.action.Update(
			datura.As[*logic.Action](artifact),
		)
	case "execution":
		crypto.execution.Update(
			datura.As[*user.Execution](artifact),
		)
	case "balances":
		crypto.balances.Update(
			datura.As[*user.Balances](artifact),
		)
	}

	return nil
}

func (crypto *Crypto) Close() error {
	crypto.cancel()
	return nil
}
