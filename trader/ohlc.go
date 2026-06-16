package trader

import (
	"context"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/symm/kraken/market"
)

type OHLC struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	ohlcs  structure.Ring[market.CandleUpdate]
}

func NewOHLC(ctx context.Context) *OHLC {
	ctx, cancel := context.WithCancel(ctx)

	ohlc := &OHLC{
		ctx:    ctx,
		cancel: cancel,
	}

	return ohlc
}

func (ohlc *OHLC) Update(update market.CandleUpdates) {
	if ohlc.ohlcs == nil {
		ohlc.ohlcs = structure.NewListRing[market.CandleUpdate](
			len(update),
			datura.Acquire("ohlc", datura.Artifact_Type_json),
		)
	}
}

func (ohlc *OHLC) Error() error {
	return ohlc.err
}

func (ohlc *OHLC) Close() error {
	ohlc.cancel()
	return nil
}
