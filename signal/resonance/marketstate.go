package resonance

import (
	"context"
	"math"
	"time"
)

type TickerSnapshot struct {
	Last      float64
	Bid       float64
	Ask       float64
	Volume    float64
	ChangePct float64
	Elapsed   float64
	Observed  time.Time
}

type marketTicker struct {
	store *feedStore
}

func newMarketTicker(ctx context.Context) *marketTicker {
	_ = ctx

	return &marketTicker{store: newFeedStore()}
}

func (ticker *marketTicker) reset() {
	if ticker == nil || ticker.store == nil {
		return
	}

	ticker.store.reset()
}

func (ticker *marketTicker) ingest(
	symbol string,
	element []byte,
	observed time.Time,
) {
	if symbol == "" || len(element) == 0 {
		return
	}

	last, lastOK := peekElementOK[float64](element, "last")
	price := last

	if !lastOK || price <= 0 {
		bid, bidOK := peekElementOK[float64](element, "bid")
		ask, askOK := peekElementOK[float64](element, "ask")

		if bidOK && askOK && ask > bid {
			price = (bid + ask) / 2
		}
	}

	timestamp, timestampOK := elementTime(element, "timestamp")

	if timestampOK {
		observed = timestamp
	}

	if observed.IsZero() {
		observed = time.Now()
	}

	ticker.store.ring(symbol).push(element, price, 0, observed)
}

func (ticker *marketTicker) Snapshot(symbol string) TickerSnapshot {
	ring := ticker.store.ring(symbol)

	if ring == nil {
		return TickerSnapshot{}
	}

	latestElement := ring.latestElement()

	if len(latestElement) == 0 {
		return TickerSnapshot{}
	}

	last, lastOK := peekElementOK[float64](latestElement, "last")
	bid, bidOK := peekElementOK[float64](latestElement, "bid")
	ask, askOK := peekElementOK[float64](latestElement, "ask")
	volume, _ := peekElementOK[float64](latestElement, "volume")
	changePct, _ := peekElementOK[float64](latestElement, "change_pct")
	window := ring.window()
	observed, observedOK := elementTime(latestElement, "timestamp")

	if !observedOK {
		observed = ring.latestObserved
	}

	if !lastOK || last <= 0 {
		return TickerSnapshot{}
	}

	snapshot := TickerSnapshot{
		Last:      last,
		Volume:    volume,
		ChangePct: changePct,
		Elapsed:   window.Elapsed,
		Observed:  observed,
	}

	if bidOK {
		snapshot.Bid = bid
	}

	if askOK {
		snapshot.Ask = ask
	}

	return snapshot
}

type marketBook struct {
	store *feedStore
}

func newMarketBook(ctx context.Context) *marketBook {
	_ = ctx

	return &marketBook{store: newFeedStore()}
}

func (book *marketBook) reset() {
	if book == nil || book.store == nil {
		return
	}

	book.store.reset()
}

func (book *marketBook) ingest(
	symbol string,
	element []byte,
	observed time.Time,
) {
	if symbol == "" || len(element) == 0 {
		return
	}

	timestamp, timestampOK := elementTime(element, "timestamp")

	if timestampOK {
		observed = timestamp
	}

	if observed.IsZero() {
		observed = time.Now()
	}

	price, spread := bookTouchMetrics(element)

	if price > 0 && spread > 0 {
		spread = (spread / price) * 10000
	}

	book.store.ring(symbol).push(element, price, spread, observed)
}

func (book *marketBook) Window(symbol string) (SymbolWindow, bool) {
	ring := book.store.ring(symbol)

	if ring == nil || ring.count == 0 {
		return SymbolWindow{}, false
	}

	return ring.window(), true
}

func (book *marketBook) Spread(symbol string) float64 {
	ring := book.store.ring(symbol)

	if ring == nil {
		return 0
	}

	latestElement := ring.latestElement()

	if len(latestElement) == 0 {
		return 0
	}

	bidPrice, bidOK := peekElementOK[float64](latestElement, "bids.0.price")
	askPrice, askOK := peekElementOK[float64](latestElement, "asks.0.price")

	if !bidOK || !askOK || bidPrice <= 0 || askPrice <= bidPrice {
		return 0
	}

	mid := (bidPrice + askPrice) / 2

	if mid <= 0 {
		return 0
	}

	return ((askPrice - bidPrice) / mid) * 10000
}

type marketTrade struct {
	store *feedStore
}

func newMarketTrade(ctx context.Context) *marketTrade {
	_ = ctx

	return &marketTrade{store: newFeedStore()}
}

func (trade *marketTrade) reset() {
	if trade == nil || trade.store == nil {
		return
	}

	trade.store.reset()
}

func (trade *marketTrade) ingest(
	symbol string,
	element []byte,
	observed time.Time,
) {
	if symbol == "" || len(element) == 0 {
		return
	}

	timestamp, timestampOK := elementTime(element, "timestamp")

	if timestampOK {
		observed = timestamp
	}

	if observed.IsZero() {
		observed = time.Now()
	}

	price, _ := peekElementOK[float64](element, "price")
	trade.store.ring(symbol).push(element, price, 0, observed)
}

func (trade *marketTrade) Window(symbol string) (SymbolWindow, bool) {
	ring := trade.store.ring(symbol)

	if ring == nil || ring.count == 0 {
		return SymbolWindow{}, false
	}

	return ring.window(), true
}

func (trade *marketTrade) Scan(symbol string, visit func(element []byte)) bool {
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

func bookTouchMetrics(element []byte) (float64, float64) {
	bidPrice, bidOK := peekElementOK[float64](element, "bids.0.price")
	askPrice, askOK := peekElementOK[float64](element, "asks.0.price")

	if !bidOK || !askOK || bidPrice <= 0 || askPrice <= bidPrice {
		return 0, 0
	}

	mid := (bidPrice + askPrice) / 2
	spread := askPrice - bidPrice

	if !finiteFloat(mid) || mid <= 0 {
		return 0, spread
	}

	return mid, spread
}

func finiteFloat(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
