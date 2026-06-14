package trader

import (
	"context"

	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/symm/kraken/market"
)

type Ticker struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	tickers structure.Ring[market.TickerUpdate]
}

func NewTicker(ctx context.Context) *Ticker {
	ctx, cancel := context.WithCancel(ctx)

	ticker := &Ticker{
		ctx:    ctx,
		cancel: cancel,
	}

	return ticker
}

func (ticker *Ticker) Update(update *market.TickerUpdates) {
	if ticker.tickers == nil {
		ticker.tickers = structure.NewListRing[market.TickerUpdate](
			len(*update),
		)
	}
}

func (ticker *Ticker) Error() error {
	return ticker.err
}

func (ticker *Ticker) Close() error {
	ticker.cancel()
	return nil
}
