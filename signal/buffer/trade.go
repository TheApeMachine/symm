package buffer

import (
	"context"
	"io"
	"time"

	"github.com/theapemachine/datura"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/signal/codec"
)

/*
Trade stores per-symbol trade elements in fixed-size rings.
*/
type Trade struct {
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	store    *feedStore
	reader   *feedReader
	Scope    string
	OnUpdate func(symbol string, element []byte)
	OnRecord func(*TradeRecord)
}

/*
NewTrade returns a trade feed buffer.
*/
func NewTrade(ctx context.Context) *Trade {
	ctx, cancel := context.WithCancel(ctx)
	store := newFeedStore("trade")

	return &Trade{
		ctx:    ctx,
		cancel: cancel,
		store:  store,
		reader: newFeedReader(store, "trade"),
	}
}

/*
Update ingests one trade artifact payload.
*/
func (trade *Trade) Update(artifact *datura.Artifact) {
	if trade == nil || artifact == nil {
		return
	}

	for _, update := range datura.As[krakenmarket.TradeUpdates](artifact) {
		trade.ingestUpdate(update)
	}
}

/*
Snapshot returns the latest trade-derived quote for one symbol.
*/
func (trade *Trade) Snapshot(symbol string) TradeSnapshot {
	ring := trade.store.ring(symbol)

	if ring == nil {
		return TradeSnapshot{}
	}

	latestElement := ring.latestElement()

	if len(latestElement) == 0 {
		return TradeSnapshot{}
	}

	price, priceOK := codec.PeekElementOK[float64](latestElement, "price")
	qty, qtyOK := codec.PeekElementOK[float64](latestElement, "qty")

	if !priceOK || !qtyOK || price <= 0 || qty <= 0 {
		return TradeSnapshot{}
	}

	window := ring.window()
	observed, observedOK := codec.ElementTime(latestElement, "timestamp")

	if !observedOK {
		observed = ring.latestObserved
	}

	volume := 0.0

	trade.Scan(symbol, func(element []byte) {
		elementPrice, elementPriceOK := codec.PeekElementOK[float64](element, "price")
		elementQty, elementQtyOK := codec.PeekElementOK[float64](element, "qty")

		if !elementPriceOK || !elementQtyOK || elementPrice <= 0 || elementQty <= 0 {
			return
		}

		volume += elementPrice * elementQty
	})

	return TradeSnapshot{
		Price:    price,
		Volume:   volume,
		Elapsed:  window.Elapsed,
		Observed: observed,
	}
}

/*
Window returns the trade history window for one symbol.
*/
func (trade *Trade) Window(symbol string) (SymbolWindow, bool) {
	ring := trade.store.ring(symbol)

	if ring == nil || ring.count == 0 {
		return SymbolWindow{}, false
	}

	return ring.window(), true
}

/*
Scan visits every stored trade element for one symbol.
*/
func (trade *Trade) Scan(symbol string, visit func(element []byte)) bool {
	if symbol == "" || visit == nil {
		return false
	}

	ring := trade.store.ring(symbol)

	if ring == nil {
		return false
	}

	elements := ring.orderedElements()

	if len(elements) == 0 {
		return false
	}

	for _, element := range elements {
		visit(element)
	}

	return true
}

/*
ResetReadHead rewinds the pipeline reader.
*/
func (trade *Trade) ResetReadHead() {
	trade.reader.scope = trade.Scope
	trade.reader.resetReadHead()
}

/*
Read streams scoped trade elements into the nomagique pipeline.
*/
func (trade *Trade) Read(buffer []byte) (int, error) {
	trade.reader.scope = trade.Scope

	return trade.reader.read(buffer)
}

/*
Error returns the last trade-buffer error.
*/
func (trade *Trade) Error() error {
	return trade.err
}

/*
Close cancels the trade buffer context.
*/
func (trade *Trade) Close() error {
	trade.cancel()

	return nil
}

func (trade *Trade) ingestUpdate(update *krakenmarket.TradeUpdate) {
	if update == nil || update.Symbol == "" {
		return
	}

	element := update.Marshal()

	if len(element) == 0 {
		return
	}

	observed := update.Timestamp

	if observed.IsZero() {
		observed = time.Now()
	}

	ring := trade.store.ring(update.Symbol)
	ring.push(element, update.Price, 0, observed)

	if trade.OnUpdate != nil {
		trade.OnUpdate(update.Symbol, element)
	}

	if trade.OnRecord != nil {
		trade.OnRecord(tradeRecordFromUpdate(update))
	}
}

func tradeRecordFromUpdate(update *krakenmarket.TradeUpdate) *TradeRecord {
	if update == nil {
		return nil
	}

	return &TradeRecord{
		Symbol:    update.Symbol,
		Side:      update.Side,
		Price:     update.Price,
		Qty:       update.Qty,
		Timestamp: update.Timestamp,
	}
}

var _ io.Reader = (*Trade)(nil)
