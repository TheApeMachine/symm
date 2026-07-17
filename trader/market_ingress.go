package trader

import (
	"context"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/websocket"
)

/*
ingressBufferSize returns the configured websocket frame buffer used to keep
Kraken's public read loop off decode and instrument enrichment work.
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
func (market *Market) bindIngress(api *websocket.API) {
	if api == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	buffer := ingressBufferSize()
	market.ctx = ctx
	market.cancel = cancel
	market.tickersIn = make(chan []byte, buffer)
	market.tradesIn = make(chan []byte, buffer)
	market.booksIn = make(chan []byte, buffer)

	go market.pump(ctx, market.tickersIn, market.OnTicker)
	go market.pump(ctx, market.tradesIn, market.OnTrade)
	go market.pump(ctx, market.booksIn, market.OnBook)

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
		case <-market.ctx.Done():
		}
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
Close stops asynchronous ingress workers started by bindIngress.
*/
func (market *Market) Close() {
	if market == nil || market.cancel == nil {
		return
	}

	market.cancel()
	market.cancel = nil
}
