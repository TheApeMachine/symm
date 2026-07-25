package manifold

import (
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
BookSource exposes SDK-managed L3 books through a read lease. PeekBook must hold
exclusion against websocket writers for the duration of fn.
*/
type BookSource interface {
	PeekBook(symbol string, fn func(*book.Book)) bool
}

/*
bookSampler reads one leased L3 book into owned manifold inputs. Tokenization and
touch scales happen under the lease so no *book.Order escapes the write barrier.
*/
type bookSampler struct {
	source BookSource
}

/*
bookPopulation is one symbol's owned sample after the lease returns.
*/
type bookPopulation struct {
	midPrice     float64
	orderIDs     []string
	batch        Batch
	reference    *decimal.Decimal
	spread       float64
	buyCapacity  *decimal.Decimal
	sellCapacity *decimal.Decimal
}

/*
newBookSampler binds the authoritative L3 source.
*/
func newBookSampler(source BookSource) *bookSampler {
	return &bookSampler{source: source}
}

/*
Sample tokenizes and measures touch for symbol under the source lease.
buyIntensity / sellIntensity become oscillator Energy on bid / ask particles.
*/
func (sampler *bookSampler) Sample(
	symbol string,
	tokenizer Tokenizer,
	buyIntensity float64,
	sellIntensity float64,
) (bookPopulation, bool) {
	if sampler == nil || sampler.source == nil || symbol == "" {
		return bookPopulation{}, false
	}

	var population bookPopulation
	var ready bool

	found := sampler.source.PeekBook(symbol, func(symbolBook *book.Book) {
		population, ready = readPopulation(
			symbolBook, tokenizer, buyIntensity, sellIntensity,
		)
	})

	return population, found && ready
}

/*
readPopulation walks one leased book into owned batch, IDs, and touch scales.
*/
func readPopulation(
	symbolBook *book.Book,
	tokenizer Tokenizer,
	buyIntensity float64,
	sellIntensity float64,
) (bookPopulation, bool) {
	if symbolBook == nil {
		return bookPopulation{}, false
	}

	bid := symbolBook.BestBid()
	ask := symbolBook.BestAsk()

	if bid == nil || ask == nil ||
		bid.Price == nil || ask.Price == nil ||
		bid.Quantity == nil || ask.Quantity == nil ||
		bid.Price.Sign() <= 0 || ask.Price.Sign() <= 0 ||
		bid.Quantity.Sign() <= 0 || ask.Quantity.Sign() <= 0 ||
		ask.Price.Cmp(bid.Price) <= 0 {
		return bookPopulation{}, false
	}

	touch := marketTouch{
		bidPrice:      bid.Price.Float64(),
		askPrice:      ask.Price.Float64(),
		bidPriceMoney: bid.Price.Copy(),
		askPriceMoney: ask.Price.Copy(),
		bidQuantity:   bid.Quantity.Copy(),
		askQuantity:   ask.Quantity.Copy(),
	}
	reference, spread, buyCapacity, sellCapacity, ok := touch.scales()

	if !ok {
		return bookPopulation{}, false
	}

	midPrice := reference.Float64()
	orderIDs := make([]string, 0)
	orders := make([]restingOrder, 0)

	appendSide := func(side *book.Side, direction book.BookDirection) {
		if side == nil {
			return
		}

		for _, level := range side.Levels {
			if level == nil {
				continue
			}

			for _, order := range level.Queue() {
				if order == nil || order.LimitPrice == nil || order.Quantity == nil ||
					order.LimitPrice.Sign() <= 0 || order.Quantity.Sign() <= 0 {
					continue
				}

				orderIDs = append(orderIDs, order.ID)
				orders = append(orders, restingOrder{
					side:  direction,
					price: order.LimitPrice.Float64(),
				})
			}
		}
	}

	appendSide(symbolBook.Bids, book.Bid)
	appendSide(symbolBook.Asks, book.Ask)

	batch := tokenizer.MakeBatch(orders, midPrice, buyIntensity, sellIntensity)

	if len(batch.Particles) == 0 {
		return bookPopulation{}, false
	}

	return bookPopulation{
		midPrice:     midPrice,
		orderIDs:     orderIDs,
		batch:        batch,
		reference:    reference,
		spread:       spread,
		buyCapacity:  buyCapacity,
		sellCapacity: sellCapacity,
	}, true
}
