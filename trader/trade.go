package trader

import (
	"context"

	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/symm/kraken/market"
)

type Trade struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	trades structure.Ring[market.TradeUpdate]
}

func NewTrade(ctx context.Context) *Trade {
	ctx, cancel := context.WithCancel(ctx)

	trade := &Trade{
		ctx:    ctx,
		cancel: cancel,
	}

	return trade
}

func (trade *Trade) Update(update *market.TradeUpdates) {
	if trade.trades == nil {
		trade.trades = structure.NewListRing[market.TradeUpdate](
			len(*update),
		)
	}
}

func (trade *Trade) Error() error {
	return trade.err
}

func (trade *Trade) Close() error {
	trade.cancel()
	return nil
}
