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
Ticker stores per-symbol ticker elements in fixed-size rings.
*/
type Ticker struct {
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	store    *feedStore
	reader   *feedReader
	Scope    string
	OnUpdate func(symbol string, element []byte)
	OnRecord func(*TickerRecord)
}

/*
NewTicker returns a ticker feed buffer.
*/
func NewTicker(ctx context.Context) *Ticker {
	ctx, cancel := context.WithCancel(ctx)
	store := newFeedStore("ticker")

	return &Ticker{
		ctx:    ctx,
		cancel: cancel,
		store:  store,
		reader: newFeedReader(store, "ticker"),
	}
}

/*
Update ingests one ticker artifact payload.
*/
func (ticker *Ticker) Update(artifact *datura.Artifact) {
	if ticker == nil || artifact == nil {
		return
	}

	for _, update := range datura.As[krakenmarket.TickerUpdates](artifact) {
		ticker.ingestUpdate(update)
	}
}

/*
Snapshot returns the latest ticker row for one symbol.
*/
func (ticker *Ticker) Snapshot(symbol string) TickerSnapshot {
	ring := ticker.store.ring(symbol)

	if ring == nil {
		return TickerSnapshot{}
	}

	latestElement := ring.latestElement()

	if len(latestElement) == 0 {
		return TickerSnapshot{}
	}

	last, lastOK := codec.PeekElementOK[float64](latestElement, "last")
	bid, bidOK := codec.PeekElementOK[float64](latestElement, "bid")
	ask, askOK := codec.PeekElementOK[float64](latestElement, "ask")
	volume, _ := codec.PeekElementOK[float64](latestElement, "volume")
	changePct, _ := codec.PeekElementOK[float64](latestElement, "change_pct")
	window := ring.window()
	observed, observedOK := codec.ElementTime(latestElement, "timestamp")

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

/*
Window returns the ticker history window for one symbol.
*/
func (ticker *Ticker) Window(symbol string) (SymbolWindow, bool) {
	ring := ticker.store.ring(symbol)

	if ring == nil || ring.count == 0 {
		return SymbolWindow{}, false
	}

	return ring.window(), true
}

/*
Scan visits every stored ticker element for one symbol.
*/
func (ticker *Ticker) Scan(symbol string, visit func(element []byte)) bool {
	if symbol == "" || visit == nil {
		return false
	}

	ring := ticker.store.ring(symbol)

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
func (ticker *Ticker) ResetReadHead() {
	ticker.reader.scope = ticker.Scope
	ticker.reader.resetReadHead()
}

/*
Read streams scoped ticker elements into the nomagique pipeline.
*/
func (ticker *Ticker) Read(buffer []byte) (int, error) {
	ticker.reader.scope = ticker.Scope

	return ticker.reader.read(buffer)
}

/*
Error returns the last ticker-buffer error.
*/
func (ticker *Ticker) Error() error {
	return ticker.err
}

/*
Close cancels the ticker buffer context.
*/
func (ticker *Ticker) Close() error {
	ticker.cancel()

	return nil
}

func (ticker *Ticker) ingestUpdate(update *krakenmarket.TickerUpdate) {
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

	price := update.Last

	if price <= 0 {
		price = update.Bid
	}

	ring := ticker.store.ring(update.Symbol)
	ring.push(element, price, 0, observed)

	if ticker.OnUpdate != nil {
		ticker.OnUpdate(update.Symbol, element)
	}

	if ticker.OnRecord != nil {
		ticker.OnRecord(tickerRecordFromUpdate(update))
	}
}

func tickerRecordFromUpdate(update *krakenmarket.TickerUpdate) *TickerRecord {
	if update == nil {
		return nil
	}

	return &TickerRecord{
		Symbol:    update.Symbol,
		Ask:       update.Ask,
		AskQty:    update.AskQty,
		Bid:       update.Bid,
		BidQty:    update.BidQty,
		Change:    update.Change,
		ChangePct: update.ChangePct,
		High:      update.High,
		Last:      update.Last,
		Low:       update.Low,
		Volume:    update.Volume,
		VWAP:      update.VWAP,
		Timestamp: update.Timestamp,
	}
}

var _ io.Reader = (*Ticker)(nil)
