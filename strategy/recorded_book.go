package strategy

import (
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/hindsight"
)

// recordedBook exposes exactly the captured touch depth. No deeper liquidity
// is manufactured. The replay workload exclusively owns this mutable cursor.
type recordedBook struct{ current *book.Book }

func (source *recordedBook) Step(observation hindsight.Observation) {
	current := &book.Book{Name: observation.Symbol, Bids: &book.Side{}, Asks: &book.Side{}}

	if observation.HasBid && observation.Bid > 0 {
		current.Bids.High = &book.Level{Price: decimal.NewFromFloat64(observation.Bid), Quantity: decimal.NewFromFloat64(observation.BidQty)}
	}

	if observation.HasAsk && observation.Ask > 0 {
		current.Asks.Low = &book.Level{Price: decimal.NewFromFloat64(observation.Ask), Quantity: decimal.NewFromFloat64(observation.AskQty)}
	}
	source.current = current
}

func (source *recordedBook) Book(symbol string, read func(*book.Book)) {
	if source.current != nil && source.current.Name == symbol {
		read(source.current)
	}
}
