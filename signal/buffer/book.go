package buffer

import (
	"context"
	"io"
	"math"
	"time"

	"github.com/theapemachine/datura"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/signal/codec"
)

/*
Book stores per-symbol book elements in fixed-size rings.
*/
type Book struct {
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	store    *feedStore
	reader   *feedReader
	Scope    string
	OnUpdate func(symbol string, element []byte)
	OnRecord func(*BookRecord)
}

/*
NewBook returns a book feed buffer.
*/
func NewBook(ctx context.Context) *Book {
	ctx, cancel := context.WithCancel(ctx)
	store := newFeedStore("book")

	return &Book{
		ctx:    ctx,
		cancel: cancel,
		store:  store,
		reader: newFeedReader(store, "book"),
	}
}

/*
Update ingests one book artifact payload.
*/
func (book *Book) Update(artifact *datura.Artifact) {
	if book == nil || artifact == nil {
		return
	}

	for _, update := range datura.As[krakenmarket.BookUpdates](artifact) {
		book.ingestUpdate(update)
	}
}

/*
Window returns the book history window for one symbol.
*/
func (book *Book) Window(symbol string) (SymbolWindow, bool) {
	ring := book.store.ring(symbol)

	if ring == nil || ring.count == 0 {
		return SymbolWindow{}, false
	}

	return ring.window(), true
}

/*
Scan visits every stored book element for one symbol.
*/
func (book *Book) Scan(symbol string, visit func(element []byte)) bool {
	if symbol == "" || visit == nil {
		return false
	}

	ring := book.store.ring(symbol)

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
Spread returns the latest touch spread in basis points for one symbol.
*/
func (book *Book) Spread(symbol string) float64 {
	ring := book.store.ring(symbol)

	if ring == nil {
		return 0
	}

	latestElement := ring.latestElement()

	if len(latestElement) == 0 {
		return 0
	}

	bidPrice, bidOK := codec.PeekElementOK[float64](latestElement, "bids.0.price")
	askPrice, askOK := codec.PeekElementOK[float64](latestElement, "asks.0.price")

	if !bidOK || !askOK || bidPrice <= 0 || askPrice <= bidPrice {
		return 0
	}

	mid := (bidPrice + askPrice) / 2

	if mid <= 0 {
		return 0
	}

	return ((askPrice - bidPrice) / mid) * 10000
}

/*
ResetReadHead rewinds the pipeline reader.
*/
func (book *Book) ResetReadHead() {
	book.reader.scope = book.Scope
	book.reader.resetReadHead()
}

/*
Read streams scoped book elements into the nomagique pipeline.
*/
func (book *Book) Read(buffer []byte) (int, error) {
	book.reader.scope = book.Scope

	return book.reader.read(buffer)
}

/*
Error returns the last book-buffer error.
*/
func (book *Book) Error() error {
	return book.err
}

/*
Close cancels the book buffer context.
*/
func (book *Book) Close() error {
	book.cancel()

	return nil
}

func (book *Book) ingestUpdate(update *krakenmarket.BookUpdate) {
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

	price, spread := bookTouchMetrics(element)
	ring := book.store.ring(update.Symbol)
	ring.push(element, price, spread, observed)

	if book.OnUpdate != nil {
		book.OnUpdate(update.Symbol, element)
	}

	if book.OnRecord != nil {
		book.OnRecord(bookRecordFromUpdate(update))
	}
}

func bookTouchMetrics(element []byte) (float64, float64) {
	bidPrice, bidOK := codec.PeekElementOK[float64](element, "bids.0.price")
	askPrice, askOK := codec.PeekElementOK[float64](element, "asks.0.price")

	if !bidOK || !askOK || bidPrice <= 0 || askPrice <= bidPrice {
		return 0, 0
	}

	mid := (bidPrice + askPrice) / 2
	spread := askPrice - bidPrice

	if !bookFiniteFloat(mid) || mid <= 0 {
		return 0, spread
	}

	return mid, spread
}

func bookFiniteFloat(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func bookRecordFromUpdate(update *krakenmarket.BookUpdate) *BookRecord {
	if update == nil {
		return nil
	}

	record := &BookRecord{
		Symbol:    update.Symbol,
		Timestamp: update.Timestamp,
		Bids:      make([]BookLevelRecord, 0, len(update.Bids)),
		Asks:      make([]BookLevelRecord, 0, len(update.Asks)),
	}

	for _, level := range update.Bids {
		record.Bids = append(record.Bids, BookLevelRecord{
			Price: level.Price,
			Qty:   level.Qty,
		})
	}

	for _, level := range update.Asks {
		record.Asks = append(record.Asks, BookLevelRecord{
			Price: level.Price,
			Qty:   level.Qty,
		})
	}

	return record
}

var _ io.Reader = (*Book)(nil)
