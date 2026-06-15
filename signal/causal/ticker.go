package causal

import (
	"context"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/symm/kraken/market"
)

type Ticker struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	entity  string
	scope   string
	symbols map[string]*tickerSymbol
}

type tickerSymbol struct {
	clock   *structure.ClockRing[struct{}]
	samples *structure.ListRing[*market.TickerUpdate]
}

func NewTicker(ctx context.Context) *Ticker {
	ctx, cancel := context.WithCancel(ctx)

	return &Ticker{
		ctx:     ctx,
		cancel:  cancel,
		entity:  "ticker",
		symbols: make(map[string]*tickerSymbol),
	}
}

func (ticker *Ticker) Entity() string {
	return ticker.entity
}

func (ticker *Ticker) Update(update *market.TickerUpdates) {
	if update == nil {
		return
	}

	for _, tickerUpdate := range *update {
		if tickerUpdate == nil || tickerUpdate.Symbol == "" {
			continue
		}

		ticker.symbols[tickerUpdate.Symbol].samples.Push(tickerUpdate)

		if !tickerUpdate.Timestamp.IsZero() {
			ticker.symbols[tickerUpdate.Symbol].clock.ObserveSecond(tickerUpdate.Timestamp)
		}
	}
}

func (ticker *Ticker) Read(p []byte) (n int, err error) {
	for range ticker.symbols {
		ticker.symbols[ticker.scope].samples.Do(func(update *market.TickerUpdate) {
			artifact := datura.Acquire("ticker", datura.Artifact_Type_json)
			artifact.WithRole("ticker")
			artifact.WithScope(update.Symbol)
			artifact.WithPayload(update.Marshal())

			n, err = artifact.Read(p)
		})
	}

	return n, err
}
