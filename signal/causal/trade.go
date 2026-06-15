package causal

import (
	"context"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/kraken/market"
)

const feedRingCapacity = 64

type Trade struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	entity  string
	scope   string
	drained bool
	symbols map[string]*tradeSymbol
}

type tradeSymbol struct {
	clock     *structure.ClockRing[struct{}]
	samples   *structure.ListRing[*market.TradeUpdate]
	nodeRing  *algorithm.NodeRing
	flow      *adaptive.Accumulator
	velocity  *adaptive.Delta
	liquidity *adaptive.EMA
	macro     *adaptive.Momentum
}

func NewTrade(ctx context.Context) *Trade {
	ctx, cancel := context.WithCancel(ctx)

	return &Trade{
		ctx:     ctx,
		cancel:  cancel,
		entity:  "trade",
		symbols: make(map[string]*tradeSymbol),
	}
}

func (trade *Trade) Entity() string {
	return trade.entity
}

func (trade *Trade) Update(update *market.TradeUpdates) {
	if update == nil {
		return
	}

	for _, tradeUpdate := range *update {
		if tradeUpdate == nil || tradeUpdate.Symbol == "" {
			continue
		}

		trade.symbols[tradeUpdate.Symbol].samples.Push(tradeUpdate)

		if !tradeUpdate.Timestamp.IsZero() {
			trade.symbols[tradeUpdate.Symbol].clock.ObserveSecond(tradeUpdate.Timestamp)
		}
	}
}

func (trade *Trade) Read(p []byte) (n int, err error) {
	for range trade.symbols {
		trade.symbols[trade.scope].samples.Do(func(update *market.TradeUpdate) {
			artifact := datura.Acquire("trade", datura.Artifact_Type_json)
			artifact.WithRole("trade")
			artifact.WithScope(update.Symbol)
			artifact.WithPayload(update.Marshal())

			n, err = artifact.Read(p)
		})
	}

	return n, err
}
