package market

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/symm/config"
)

// feedReconnectDelay is how long a shared feed waits before reopening its
// upstream after the connection drops.
const feedReconnectDelay = time.Second

// feedBuffer is the per-subscriber buffer; a subscriber that falls behind drops
// frames rather than stalling the whole fan-out.
const feedBuffer = 256

/*
feed multiplexes one upstream Kraken stream to many in-process subscribers. Every
signal that watches the same universe shares a single connection instead of each
dialing its own, which is what keeps the connection count flat (one per channel)
no matter how many signals run. The upstream opens once on the first subscriber,
runs for the life of the process, and auto-reconnects if it drops.
*/
type feed[T any] struct {
	mu   sync.Mutex
	subs map[chan *T]struct{}
	dial func() <-chan *T
}

func newFeed[T any]() *feed[T] {
	return &feed[T]{subs: make(map[chan *T]struct{})}
}

/*
subscribe attaches a consumer and returns its channel. The first caller's dial
closure defines how the shared upstream is (re)opened; later callers reuse the
already-running upstream. The consumer is detached when ctx is canceled.
*/
func (sharedFeed *feed[T]) subscribe(
	ctx context.Context, dial func() <-chan *T,
) <-chan *T {
	out := make(chan *T, feedBuffer)

	sharedFeed.mu.Lock()
	first := sharedFeed.dial == nil

	if first {
		sharedFeed.dial = dial
	}

	sharedFeed.subs[out] = struct{}{}
	sharedFeed.mu.Unlock()

	if first {
		go sharedFeed.run()
	}

	go sharedFeed.detach(ctx, out)

	return out
}

// detach removes a consumer once its context is canceled and closes its channel
// so its range loop ends. The shared upstream keeps running for the others.
func (sharedFeed *feed[T]) detach(ctx context.Context, out chan *T) {
	<-ctx.Done()

	sharedFeed.mu.Lock()
	delete(sharedFeed.subs, out)
	sharedFeed.mu.Unlock()

	close(out)
}

// run owns the upstream: it reads until the connection drops, then waits and
// reopens. It lives for the life of the process.
func (sharedFeed *feed[T]) run() {
	for {
		for value := range sharedFeed.dial() {
			sharedFeed.fanout(value)
		}

		if replayActive() && !config.System.ReplayLoop {
			return
		}

		time.Sleep(feedReconnectDelay)
	}
}

func replayActive() bool {
	return strings.TrimSpace(config.System.ReplayFile) != ""
}

// fanout copies one upstream value to every attached subscriber, dropping for
// any subscriber whose buffer is full so one slow signal cannot stall the rest.
func (sharedFeed *feed[T]) fanout(value *T) {
	sharedFeed.mu.Lock()
	defer sharedFeed.mu.Unlock()

	for sub := range sharedFeed.subs {
		select {
		case sub <- value:
		default:
		}
	}
}

// The shared feeds for the high-fan-out public channels. The trade/ticker/book
// streams are consumed by many signals over the same universe, so they are
// multiplexed; OHLC is per-symbol (anchor plus open positions) and dials directly.
var (
	tradeFeed  = newFeed[TradeUpdate]()
	tickerFeed = newFeed[TickerUpdate]()
	bookFeed   = newFeed[BookUpdate]()
)
