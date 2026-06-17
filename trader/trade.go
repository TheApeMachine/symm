package trader

import (
	"context"

	"github.com/theapemachine/datura"
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

func (trade *Trade) Update(artifact *datura.Artifact) {
	updates := datura.As[market.TradeUpdates](artifact)

	if trade.trades == nil {
		trade.trades = structure.NewListRing[market.TradeUpdate](
			len(updates),
			datura.Acquire("trade", datura.Artifact_Type_json),
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
