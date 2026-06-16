package prediction

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

const feedRingCapacity = 64

type TradeSeries struct {
	Prices  []float64
	Volumes []float64
	At      time.Time
}

type Trade struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	entity  string
	scope   string
	symbols *sync.Map
}

func NewTrade(ctx context.Context) *Trade {
	ctx, cancel := context.WithCancel(ctx)

	return &Trade{
		ctx:     ctx,
		cancel:  cancel,
		entity:  "trade",
		symbols: &sync.Map{},
	}
}

func (trade *Trade) Update(update krakenmarket.TradeUpdates) {
	for _, tradeUpdate := range update {
		if tradeUpdate == nil || tradeUpdate.Symbol == "" {
			continue
		}

		ring, _ := trade.symbols.LoadOrStore(
			tradeUpdate.Symbol,
			structure.NewListRing[*krakenmarket.TradeUpdate](
				feedRingCapacity,
				datura.Acquire(
					"prediction", datura.Artifact_Type_json,
				).WithRole("trade"),
			),
		)

		ring.(*structure.ListRing[*krakenmarket.TradeUpdate]).Push(tradeUpdate)
	}
}

func (trade *Trade) Series(symbol string) TradeSeries {
	value, ok := trade.symbols.Load(symbol)

	if !ok {
		return TradeSeries{}
	}

	ring := value.(*structure.ListRing[*krakenmarket.TradeUpdate])

	var (
		prices  []float64
		volumes []float64
		latest  *krakenmarket.TradeUpdate
	)

	ring.Do(func(update *krakenmarket.TradeUpdate) {
		if update == nil {
			return
		}

		if update.Price <= 0 || update.Qty <= 0 {
			return
		}

		prices = append(prices, update.Price)
		volumes = append(volumes, update.Qty)
		latest = update
	})

	if latest == nil {
		return TradeSeries{}
	}

	return TradeSeries{
		Prices:  prices,
		Volumes: volumes,
		At:      latest.Timestamp,
	}
}
