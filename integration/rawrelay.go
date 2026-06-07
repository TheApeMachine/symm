package integration

import (
	"context"
	"time"

	"github.com/theapemachine/qpool"
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
	ctx       context.Context
	cancel    context.CancelFunc
	raw       *qpool.BroadcastGroup
	consumers []*qpool.BroadcastConsumer
}

func NewRawRelay(ctx context.Context, pool *qpool.Q[any]) *RawRelay {
	ctx, cancel := context.WithCancel(ctx)

	raw := pool.CreateBroadcastGroup("raw", 10*time.Millisecond)

	if raw == nil {
		cancel()
		return nil
	}

	relay := &RawRelay{
		ctx:    ctx,
		cancel: cancel,
		raw:    raw,
	}

	for _, channel := range replayRelayChannels {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)

		if group == nil {
			cancel()
			return nil
		}
		consumer := group.Subscribe("integration/rawrelay:"+channel, 4096)

		if consumer == nil {
			cancel()
			return nil
		}

		relay.consumers = append(relay.consumers, consumer)
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
	for _, consumer := range relay.consumers {
		message, err := consumer.Wait(relay.ctx)
		if err != nil {
			return false
		}

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
	}

	return false
}

func (relay *RawRelay) Close() error {
	relay.cancel()

	return nil
}
