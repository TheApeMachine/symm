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
sharedFeed multiplexes one upstream Kraken stream to many in-process subscribers.
The upstream opens when the first subscriber attaches, uses the union of every
active subscriber's symbols (and the max book depth), and stops when the last
subscriber detaches.
*/
type subscriptionEntry struct {
	spec subscriptionSpec
	stop chan struct{}
}

type sharedFeed[T any] struct {
	mu             sync.Mutex
	fanoutMu       sync.Mutex
	subs           map[chan *T]subscriptionEntry
	feedCtx        context.Context
	feedCancel     context.CancelFunc
	upstreamCancel context.CancelFunc
	activeSpec     subscriptionSpec
	running        bool
	reliable       bool
	dial           func(ctx context.Context, spec subscriptionSpec) <-chan *T
}

func newSharedFeed[T any](
	dial func(ctx context.Context, spec subscriptionSpec) <-chan *T,
) *sharedFeed[T] {
	return &sharedFeed[T]{
		subs: make(map[chan *T]subscriptionEntry),
		dial: dial,
	}
}

func newReliableSharedFeed[T any](
	dial func(ctx context.Context, spec subscriptionSpec) <-chan *T,
) *sharedFeed[T] {
	feed := newSharedFeed(dial)
	feed.reliable = true

	return feed
}

/*
subscribe attaches a consumer and returns its channel. The shared upstream uses
the union of every active subscriber's symbols. The consumer is detached when ctx
is canceled; the upstream stops when the last consumer detaches.
*/
func (sharedFeed *sharedFeed[T]) subscribe(
	ctx context.Context, spec subscriptionSpec,
) <-chan *T {
	out := make(chan *T, feedBuffer)
	bridge := out

	if sharedFeed.reliable {
		bridge = make(chan *T, feedBuffer)

		go func() {
			defer close(out)

			for value := range bridge {
				out <- value
			}
		}()
	}

	stop := make(chan struct{})

	sharedFeed.mu.Lock()
	sharedFeed.subs[bridge] = subscriptionEntry{spec: spec, stop: stop}
	merged := sharedFeed.mergedSpecLocked()
	sharedFeed.ensureRunningLocked(merged)
	sharedFeed.mu.Unlock()

	go sharedFeed.detach(ctx, bridge, out)

	return out
}

func (sharedFeed *sharedFeed[T]) mergedSpecLocked() subscriptionSpec {
	specs := make([]subscriptionSpec, 0, len(sharedFeed.subs))

	for _, entry := range sharedFeed.subs {
		specs = append(specs, entry.spec)
	}

	return mergeSubscriptionSpecs(specs)
}

func (sharedFeed *sharedFeed[T]) ensureRunningLocked(merged subscriptionSpec) {
	if !sharedFeed.running {
		sharedFeed.feedCtx, sharedFeed.feedCancel = context.WithCancel(context.Background())
		sharedFeed.running = true
		sharedFeed.activeSpec = merged

		go sharedFeed.run()

		return
	}

	if !subscriptionSpecEqual(sharedFeed.activeSpec, merged) {
		sharedFeed.activeSpec = merged
		sharedFeed.restartUpstreamLocked()
	}
}

func (sharedFeed *sharedFeed[T]) restartUpstreamLocked() {
	if sharedFeed.upstreamCancel != nil {
		sharedFeed.upstreamCancel()
		sharedFeed.upstreamCancel = nil
	}
}

func (sharedFeed *sharedFeed[T]) detach(ctx context.Context, bridge, out chan *T) {
	<-ctx.Done()

	reliable := sharedFeed.reliable

	sharedFeed.mu.Lock()
	entry, subscribed := sharedFeed.subs[bridge]

	if subscribed {
		delete(sharedFeed.subs, bridge)
		close(entry.stop)
	}

	if len(sharedFeed.subs) == 0 {
		sharedFeed.stopLocked()
	} else if subscribed {
		merged := sharedFeed.mergedSpecLocked()

		if !subscriptionSpecEqual(sharedFeed.activeSpec, merged) {
			sharedFeed.activeSpec = merged
			sharedFeed.restartUpstreamLocked()
		}
	}

	sharedFeed.mu.Unlock()

	if reliable {
		sharedFeed.fanoutMu.Lock()
		close(bridge)
		sharedFeed.fanoutMu.Unlock()

		return
	}

	close(bridge)

	if bridge == out {
		return
	}
}

func (sharedFeed *sharedFeed[T]) stopLocked() {
	sharedFeed.restartUpstreamLocked()

	if sharedFeed.feedCancel != nil {
		sharedFeed.feedCancel()
		sharedFeed.feedCancel = nil
	}

	sharedFeed.running = false
}

func (sharedFeed *sharedFeed[T]) run() {
	for {
		sharedFeed.mu.Lock()
		feedCtx := sharedFeed.feedCtx
		sharedFeed.mu.Unlock()

		if feedCtx == nil {
			return
		}

		select {
		case <-feedCtx.Done():
			return
		default:
		}

		spec := sharedFeed.currentSpec()
		upstreamCtx, cancel := context.WithCancel(feedCtx)

		sharedFeed.mu.Lock()
		sharedFeed.upstreamCancel = cancel
		sharedFeed.mu.Unlock()

		for value := range sharedFeed.dial(upstreamCtx, spec) {
			sharedFeed.fanout(value)
		}

		cancel()

		sharedFeed.mu.Lock()
		feedCtx = sharedFeed.feedCtx
		empty := len(sharedFeed.subs) == 0
		sharedFeed.mu.Unlock()

		if feedCtx == nil || feedCtx.Err() != nil {
			return
		}

		if replayActive() && !config.System.ReplayLoop {
			return
		}

		if empty {
			return
		}

		time.Sleep(feedReconnectDelay)
	}
}

func (sharedFeed *sharedFeed[T]) currentSpec() subscriptionSpec {
	sharedFeed.mu.Lock()
	defer sharedFeed.mu.Unlock()

	return sharedFeed.activeSpec
}

func replayActive() bool {
	return strings.TrimSpace(config.System.ReplayFile) != ""
}

func (sharedFeed *sharedFeed[T]) fanout(value *T) {
	if sharedFeed.reliable {
		sharedFeed.fanoutReliable(value)

		return
	}

	sharedFeed.fanoutDrop(value)
}

func (sharedFeed *sharedFeed[T]) fanoutDrop(value *T) {
	sharedFeed.mu.Lock()
	defer sharedFeed.mu.Unlock()

	for sub := range sharedFeed.subs {
		select {
		case sub <- value:
		default:
		}
	}
}

func (sharedFeed *sharedFeed[T]) fanoutReliable(value *T) {
	sharedFeed.fanoutMu.Lock()
	defer sharedFeed.fanoutMu.Unlock()

	sharedFeed.mu.Lock()
	bridges := make([]chan *T, 0, len(sharedFeed.subs))

	for bridge := range sharedFeed.subs {
		bridges = append(bridges, bridge)
	}

	sharedFeed.mu.Unlock()

	for _, bridge := range bridges {
		sharedFeed.deliverReliable(bridge, value)
	}
}

func (sharedFeed *sharedFeed[T]) deliverReliable(bridge chan *T, value *T) {
	sharedFeed.mu.Lock()
	entry, subscribed := sharedFeed.subs[bridge]
	feedCtx := sharedFeed.feedCtx
	sharedFeed.mu.Unlock()

	if !subscribed {
		return
	}

	stop := entry.stop

	if feedCtx == nil {
		select {
		case bridge <- value:
		case <-stop:
		}

		return
	}

	select {
	case bridge <- value:
	case <-stop:
	case <-feedCtx.Done():
	}
}

var (
	tradeFeed = newSharedFeed(func(ctx context.Context, spec subscriptionSpec) <-chan *TradeUpdate {
		return tradeUpstream(ctx, spec.symbols)
	})
	tickerFeed = newSharedFeed(func(ctx context.Context, spec subscriptionSpec) <-chan *TickerUpdate {
		return tickerUpstream(ctx, spec.symbols)
	})
	bookFeed = newReliableSharedFeed(func(ctx context.Context, spec subscriptionSpec) <-chan *BookUpdate {
		return bookUpstream(ctx, spec.depth, spec.symbols)
	})
)
