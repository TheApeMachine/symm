package manifold

import (
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
bookSampler owns the converted L3 population for each symbol. SDK decimal
objects are immutable snapshots that writers replace on modification, so
pointer identity lets unchanged orders reuse their float and exact-money
representations instead of rebuilding big.Rat values on every analyzer tick.
*/
type bookSampler struct {
	source  BookSource
	samples map[string]*bookSample
}

/*
bookSample retains only the current live order identities for one symbol.
Orders is reused between sequential solver updates; cache entries not observed
in the current leased book are removed immediately.
*/
type bookSample struct {
	epoch   uint64
	orders  []physicalOrder
	cache   map[string]cachedPhysicalOrder
	bestBid float64
	bestAsk float64
	read    func(*book.Book)
}

/*
cachedPhysicalOrder binds one converted order to the SDK decimal objects from
which it was derived. A quantity modification replaces its source pointer and
therefore refreshes the cached representation on the next sample.
*/
type cachedPhysicalOrder struct {
	priceSource    *decimal.Decimal
	quantitySource *decimal.Decimal
	observed       uint64
	order          physicalOrder
}

/*
newBookSampler composes a bounded converter around the authoritative L3 source.
*/
func newBookSampler(source BookSource) *bookSampler {
	return &bookSampler{
		source:  source,
		samples: make(map[string]*bookSample),
	}
}

/*
Orders returns the current complete two-sided population for symbol. Conversion
runs under the source lease only for orders whose SDK decimal snapshot changed.
*/
func (sampler *bookSampler) Orders(
	symbol string,
) ([]physicalOrder, float64, bool) {
	if sampler == nil || sampler.source == nil || symbol == "" {
		return nil, 0, false
	}

	sample := sampler.samples[symbol]

	if sample == nil {
		sample = &bookSample{cache: make(map[string]cachedPhysicalOrder)}
		sample.read = sample.capture
		sampler.samples[symbol] = sample
	}

	found := sampler.source.PeekBook(symbol, sample.read)

	if !found {
		delete(sampler.samples, symbol)
		return nil, 0, false
	}

	if len(sample.orders) == 0 ||
		sample.bestBid <= 0 || sample.bestAsk <= sample.bestBid {
		return nil, 0, false
	}

	return sample.orders, (sample.bestBid + sample.bestAsk) / 2, true
}

/*
capture refreshes one sample while the BookSource read lease is held.
*/
func (sample *bookSample) capture(symbolBook *book.Book) {
	sample.epoch++
	sample.orders = sample.orders[:0]
	sample.bestBid = 0
	sample.bestAsk = 0

	if symbolBook != nil {
		sample.captureSide(symbolBook.Bids)
		sample.captureSide(symbolBook.Asks)
	}

	for orderID, cached := range sample.cache {
		if cached.observed != sample.epoch {
			delete(sample.cache, orderID)
		}
	}
}

/*
captureSide walks one leased SDK side and appends each resting order once.
*/
func (sample *bookSample) captureSide(side *book.Side) {
	if side == nil {
		return
	}

	for _, level := range side.Levels {
		if level == nil {
			continue
		}

		for _, order := range level.Queue() {
			physical, ready := sample.convert(side.Direction, order)

			if !ready {
				continue
			}

			sample.observeTouch(physical)
			sample.orders = append(sample.orders, physical)
		}
	}
}

/*
convert reuses one unchanged order or converts its replacement decimals once.
*/
func (sample *bookSample) convert(
	direction book.BookDirection,
	order *book.Order,
) (physicalOrder, bool) {
	if order == nil || order.ID == "" ||
		order.Quantity == nil || order.LimitPrice == nil {
		return physicalOrder{}, false
	}

	cached, exists := sample.cache[order.ID]

	if exists && cached.priceSource == order.LimitPrice &&
		cached.quantitySource == order.Quantity && cached.order.side == direction {
		cached.observed = sample.epoch
		sample.cache[order.ID] = cached
		return cached.order, true
	}

	physical := physicalOrder{
		orderID:       order.ID,
		side:          direction,
		price:         order.LimitPrice.Float64(),
		quantity:      order.Quantity.Float64(),
		priceMoney:    order.LimitPrice.Copy(),
		quantityMoney: order.Quantity.Copy(),
		timestamp:     order.Timestamp,
	}

	if physical.price <= 0 || physical.quantity <= 0 {
		return physicalOrder{}, false
	}

	sample.cache[order.ID] = cachedPhysicalOrder{
		priceSource:    order.LimitPrice,
		quantitySource: order.Quantity,
		observed:       sample.epoch,
		order:          physical,
	}

	return physical, true
}

/*
observeTouch derives the best bid and ask from the same converted population.
*/
func (sample *bookSample) observeTouch(order physicalOrder) {
	if order.side == book.Bid && order.price > sample.bestBid {
		sample.bestBid = order.price
	}

	if order.side == book.Ask &&
		(sample.bestAsk == 0 || order.price < sample.bestAsk) {
		sample.bestAsk = order.price
	}
}
