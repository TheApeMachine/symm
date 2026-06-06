package integration

import (
	"context"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/kraken/public"
)

var replayRelayChannels = []string{
	public.TickerChannel,
	public.BookChannel,
	public.TradesChannel,
	public.CandlesChannel,
	public.InstrumentsChannel,
	"level3",
}

/*
RawRelay forwards replay websocket channel frames onto the shared raw bus.
*/
type RawRelay struct {
	ctx         context.Context
	cancel      context.CancelFunc
	raw         *qpool.BroadcastGroup
	subscribers []*qpool.Subscriber
}

func NewRawRelay(ctx context.Context, pool *qpool.Q) *RawRelay {
	ctx, cancel := context.WithCancel(ctx)

	relay := &RawRelay{
		ctx:    ctx,
		cancel: cancel,
		raw:    bus.Group(pool, "raw", 10*time.Millisecond),
	}

	for _, channel := range replayRelayChannels {
		group := bus.Group(pool, channel, 10*time.Millisecond)
		subscriber := group.Subscribe("integration/rawrelay:"+channel, 4096)

		if subscriber == nil {
			cancel()
			return nil
		}

		relay.subscribers = append(relay.subscribers, subscriber)
	}

	return relay
}

func (relay *RawRelay) Tick() error {
	for {
		select {
		case <-relay.ctx.Done():
			return relay.ctx.Err()
		default:
		}

		forwarded := relay.forwardOne()

		if !forwarded {
			select {
			case <-relay.ctx.Done():
				return relay.ctx.Err()
			case <-time.After(2 * time.Millisecond):
			}
		}
	}
}

func (relay *RawRelay) forwardOne() bool {
	for _, subscriber := range relay.subscribers {
		select {
		case message := <-subscriber.Incoming:
			if message == nil || message.Value == nil {
				continue
			}

			envelope, ok := message.Value.(map[string]any)

			if !ok {
				continue
			}

			ch, _ := envelope["channel"].(string)
			relay.raw.Send(&qpool.QValue[any]{
				Type:  ch,
				Value: envelope,
			})

			return true
		default:
		}
	}

	return false
}

func (relay *RawRelay) Close() error {
	relay.cancel()

	return nil
}
