package trader

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/rawbus"
)

type Crypto struct {
	ctx        context.Context
	cancel     context.CancelFunc
	pool       *qpool.Q[any]
	ui         *qpool.BroadcastGroup
	bus        *internal.Bus
	instrument *Instrument
	ohlc       *OHLC
	action     *Action
}

func NewCrypto(
	ctx context.Context, pool *qpool.Q[any],
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	bus := internal.NewBus(
		ctx,
		pool,
		[]internal.Channel{
			internal.ChannelKrakenPublic,
			internal.ChannelKrakenPrivate,
			internal.ChannelKrakenFutures,
			internal.ChannelUI,
			internal.ChannelRaw,
		},
		[]internal.Subscription{
			internal.Subscribe(internal.ChannelRaw, "trader:crypto"),
		},
	)

	return &Crypto{
		ctx:        ctx,
		cancel:     cancel,
		pool:       pool,
		ui:         pool.CreateBroadcastGroup("ui", 10*time.Millisecond),
		bus:        bus,
		instrument: NewInstrument(ctx, bus),
		ohlc:       NewOHLC(ctx, bus),
		action:     NewAction(ctx, bus),
	}
}

/*
Tick consumes the raw bus. Story emits actions; crypto forwards them as order
messages on raw for the desk, and handles instruments and subscriptions here.
*/
func (crypto *Crypto) Tick() (err error) {
	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		default:
			message, err := crypto.bus.Receive(internal.ChannelRaw)

			if internal.IsShutdown(err) {
				return err
			}

			if internal.ReportError(err) != nil || message == nil {
				continue
			}

			switch rawbus.TypeFrom(message.Type) {
			case rawbus.TypeReconnect:
				crypto.reset()
			case rawbus.TypeInstrument:
				if err := crypto.instrument.Tick(message); err != nil {
					return errnie.Err(
						errnie.IO,
						"crypto: failed to tick instrument",
						err,
					)
				}
			case rawbus.TypeOHLC:
				if err := crypto.ohlc.Tick(message); err != nil {
					return errnie.Err(
						errnie.IO,
						"crypto: failed to tick ohlc",
						err,
					)
				}
			case rawbus.TypeBalances:
				balances, ok := message.Value.(user.Balances)

				if !ok {
					continue
				}

				if err := crypto.instrument.SubscribePositionCandles(balances); err != nil {
					return errnie.Err(
						errnie.IO,
						"crypto: failed to subscribe position ohlc",
						err,
					)
				}
			case rawbus.TypeActions:
				if err := crypto.action.Tick(message); err != nil {
					return errnie.Err(
						errnie.IO,
						"crypto: failed to tick action",
						err,
					)
				}
			}
		}
	}
}

func (crypto *Crypto) reset() {
	crypto.instrument.reset()
}

func (crypto *Crypto) Close() error {
	if crypto.cancel != nil {
		crypto.cancel()
	}

	return nil
}
