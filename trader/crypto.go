package trader

import (
	"context"
	"errors"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/logic"
)

type Crypto struct {
	ctx    context.Context
	cancel context.CancelFunc
	pool   *qpool.Q[any]
	ui     *qpool.BroadcastGroup
	bus    *internal.Bus
	desk   *broker.Desk
}

func NewCrypto(ctx context.Context, pool *qpool.Q[any]) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	return &Crypto{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		ui:     pool.CreateBroadcastGroup("ui", 10*time.Millisecond),
		bus: internal.NewBus(
			ctx,
			pool,
			[]string{"kraken:public", "ui"},
			[]string{"raw"},
		),
		desk: broker.NewDesk(ctx, pool),
	}
}

/*
Tick consumes the raw bus. Story emits Action structs (entry/exit verdicts) and
the paper desk emits execution maps. Actions submit orders; executions update
inventory and the shared focus set so the not-holding gate sees open positions.
*/
func (crypto *Crypto) Tick() (err error) {
	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		default:
			message, err := crypto.bus.Receive("raw")

			if errnie.Error(err) != nil || message == nil {
				continue
			}

			var (
				ok bool
			)

			switch message.Type {
			case "instrument":
				var (
					instrument *market.InstrumentUpdate
				)

				if instrument, ok = message.Value.(*market.InstrumentUpdate); !ok {
					errnie.Error(errors.New("crypto: invalid instrument"), "crypto: invalid instrument")
					continue
				}

				pairs := make([]string, 0)

				for _, pair := range instrument.Pairs {
					if pair.Quote == viper.GetString("market.quote_currency") {
						pairs = append(pairs, pair.Symbol)
					}
				}

				crypto.bus.Send("kraken:public", "ticker", types.KrakenMessage{
					Method: "subscribe",
					Params: market.NewTickerParams(pairs),
					ReqID:  time.Now().UnixNano(),
				})

				crypto.bus.Send("kraken:public", "book", types.KrakenMessage{
					Method: "subscribe",
					Params: market.NewBookParams(pairs, viper.GetInt("market.book_depth_levels")),
					ReqID:  time.Now().UnixNano(),
				})

				crypto.bus.Send("kraken:public", "trade", types.KrakenMessage{
					Method: "subscribe",
					Params: market.NewTradeParams(pairs),
					ReqID:  time.Now().UnixNano(),
				})
			case "actions":
				var (
					action *logic.Action
				)

				if action, ok = message.Value.(*logic.Action); !ok {
					errnie.Error(errors.New("crypto: invalid action"), "crypto: invalid action")
					continue
				}

				crypto.desk.AddOrder(action)
			}
		}
	}
}

func (crypto *Crypto) Close() error {
	if crypto.cancel != nil {
		crypto.cancel()
	}

	return nil
}
