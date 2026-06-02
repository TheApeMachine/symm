package paper

import (
	"context"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
)

/*
Prices tracks last mid prices from the shared raw market bus.
*/
type Prices struct {
	ctx       context.Context
	pool      *qpool.Q
	mu        sync.RWMutex
	lastPrice map[string]float64
}

func NewPrices(ctx context.Context, pool *qpool.Q) *Prices {
	return &Prices{
		ctx:       ctx,
		pool:      pool,
		lastPrice: make(map[string]float64),
	}
}

func (prices *Prices) Run() {
	group := prices.pool.CreateBroadcastGroup("raw", 10*time.Millisecond)
	subscriber := group.Subscribe("paper:prices", 128)

	for {
		select {
		case <-prices.ctx.Done():
			return
		case message, ok := <-subscriber.Incoming:
			if !ok || message == nil || message.Value == nil {
				continue
			}

			envelope, ok := message.Value.(public.SocketMessage)

			if !ok || envelope.Channel != public.TickerChannel {
				continue
			}

			prices.ingest(envelope)
		}
	}
}

func (prices *Prices) ingest(envelope public.SocketMessage) {
	var rows []struct {
		Symbol string  `json:"symbol"`
		Bid    float64 `json:"bid"`
		Ask    float64 `json:"ask"`
		Last   float64 `json:"last"`
	}

	if err := sonic.Unmarshal(envelope.Data, &rows); err != nil {
		return
	}

	prices.mu.Lock()
	defer prices.mu.Unlock()

	for _, row := range rows {
		if row.Symbol == "" {
			continue
		}

		mid := row.Last

		if row.Bid > 0 && row.Ask > 0 {
			mid = (row.Bid + row.Ask) / 2.0
		}

		if mid > 0 {
			prices.lastPrice[row.Symbol] = mid
		}
	}
}

func (prices *Prices) Last(symbol string) float64 {
	prices.mu.RLock()
	defer prices.mu.RUnlock()

	return prices.lastPrice[symbol]
}
