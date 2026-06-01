package market

import (
	"context"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
)

/*
Feed is a subscribed Kraken WebSocket channel. Callers range Stream and decode
rows from each SocketMessage.
*/
type Feed struct {
	Stream <-chan *public.SocketMessage
}

/*
OpenFeed broadcasts a subscribe request on kraken:public and subscribes to the
data channel. No client needed — pool handles everything.
*/
func OpenFeed(ctx context.Context, pool *qpool.Q, channel string, params any) Feed {
	outbound := pool.CreateBroadcastGroup("kraken:public", 10*time.Millisecond)
	outbound.Send(&qpool.QValue[any]{Value: map[string]any{
		"method": "subscribe",
		"params": params,
	}})

	group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
	sub := group.Subscribe(channel, 128)

	out := make(chan *public.SocketMessage, 128)

	go func() {
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-sub.Incoming:
				if !ok {
					return
				}

				if msg == nil {
					continue
				}

				sm, ok := msg.Value.(public.SocketMessage)

				if !ok {
					continue
				}

				row := sm

				select {
				case out <- &row:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return Feed{Stream: out}
}
