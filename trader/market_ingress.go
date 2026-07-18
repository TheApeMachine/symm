package trader

import (
	"context"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
)

const (
	bookResyncCooldown = time.Second
	bookResyncAttempts = 3
)

/*
ingressBufferSize returns the configured websocket frame buffer used to keep
Kraken's public read loop off decode and instrument enrichment work.

ponytail: the fixed 64-frame minimum is an intentional simplification with
memory and throughput limits; workload-aware per-channel sizing is the upgrade path.
*/
func ingressBufferSize() int {
	buffer := viper.GetInt("system.websocket.channel.buffer")

	if buffer < 64 {
		return 64
	}

	return buffer
}

/*
bindIngress registers public handlers that only enqueue raw frames. Decode and
Observe run on dedicated workers so book pressure cannot starve trade reads.
*/
func (market *Market) bindIngress(parent context.Context, api *websocket.API) {
	if api == nil {
		return
	}

	ctx, cancel := context.WithCancel(parent)
	buffer := ingressBufferSize()
	market.ctx = ctx
	market.cancel = cancel
	market.tickersIn = make(chan []byte, buffer)
	market.tradesIn = make(chan []byte, buffer)
	market.booksIn = make(chan []byte, buffer)
	market.resyncIn = make(chan string, buffer)

	go market.pump(ctx, market.tickersIn, market.OnTicker)
	go market.pump(ctx, market.tradesIn, market.OnTrade)
	go market.pump(ctx, market.booksIn, market.OnBook)
	go market.resyncWorker(ctx)

	api.On("ticker", market.offer(market.tickersIn))
	api.On("trade", market.offer(market.tradesIn))
	api.On("book", market.offer(market.booksIn))
}

/*
offer copies one websocket payload into an ingress queue. A full queue applies
backpressure to the reader instead of silently discarding the frame.
*/
func (market *Market) offer(queue chan []byte) func([]byte) {
	return func(data []byte) {
		if len(data) == 0 {
			return
		}

		select {
		case queue <- append([]byte(nil), data...):
			market.dirtyWake()
		case <-market.ctx.Done():
		}
	}
}

/*
dirtyWake signals that ingress arrived so the runtime can coalesce one Cut.
*/
func (market *Market) dirtyWake() {
	if market == nil || market.dirty == nil {
		return
	}

	select {
	case market.dirty <- struct{}{}:
	default:
	}
}

/*
WaitDirty blocks until ingress dirties the market, the budget elapses, or the
market context cancels.
*/
func (market *Market) WaitDirty(budget time.Duration) {
	if market == nil {
		return
	}

	if budget <= 0 {
		budget = 10 * time.Millisecond
	}

	var done <-chan struct{}

	if market.ctx != nil {
		done = market.ctx.Done()
	}

	var dirty <-chan struct{}

	if market.dirty != nil {
		dirty = market.dirty
	}

	select {
	case <-done:
	case <-dirty:
	case <-time.After(budget):
	}
}

/*
pump drains one ingress queue until Market.Close cancels the shared context.
*/
func (market *Market) pump(
	ctx context.Context,
	queue <-chan []byte,
	handle func([]byte),
) {
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-queue:
			handle(data)
		}
	}
}

/*
scheduleBookResync enqueues one symbol for coalesced book resnapshot work.
When market.api is nil the call is a no-op.
*/
func (market *Market) scheduleBookResync(symbol string) {
	if market == nil || market.api == nil || market.resyncIn == nil || symbol == "" {
		return
	}

	select {
	case market.resyncIn <- symbol:
	case <-market.ctx.Done():
	default:
	}
}

/*
resyncWorker coalesces failed book symbols, retries ResyncBook with cooldown,
and exits when Market.Close cancels ingress.
*/
func (market *Market) resyncWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case symbol := <-market.resyncIn:
			pending := map[string]struct{}{symbol: {}}

		drain:
			for {
				select {
				case next := <-market.resyncIn:
					pending[next] = struct{}{}
				default:
					break drain
				}
			}

			symbols := make([]string, 0, len(pending))

			for pendingSymbol := range pending {
				symbols = append(symbols, pendingSymbol)
			}

			for attempt := 0; attempt < bookResyncAttempts; attempt++ {
				resyncErr := market.api.ResyncBook(symbols)

				if resyncErr == nil {
					break
				}

				errnie.Error(errnie.Err(
					errnie.Internal,
					"market: resync book snapshot",
					resyncErr,
				))

				select {
				case <-ctx.Done():
					return
				case <-time.After(bookResyncCooldown):
				}
			}
		}
	}
}

/*
Close stops asynchronous ingress workers started by bindIngress.
*/
func (market *Market) Close() {
	if market == nil || market.cancel == nil {
		return
	}

	market.cancel()
	market.cancel = nil
}
