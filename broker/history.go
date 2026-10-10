package broker

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
)

/*
BookHistory answers what a symbol's book displayed at a venue time, so a
signal reads the book as of its frame rather than whatever the live book has
moved on to by the time the frame is processed.
*/
type BookHistory interface {
	BookAt(symbol string, at time.Time, read func(*BookView))
}

/*
BookLevel is one aggregated price level.
*/
type BookLevel struct {
	Price    float64
	Quantity float64
}

/*
BookView is one immutable, checksum-verified version of a symbol's book: the
state after the venue frame stamped At, in effect until the next version.
Levels run best first. FullBid/FullAsk report a side holding its configured
depth, beyond which levels are hidden. Complete is false when a level lacked
a price or quantity; such a level is not listed.
*/
type BookView struct {
	At       time.Time
	Bids     []BookLevel
	Asks     []BookLevel
	FullBid  bool
	FullAsk  bool
	Complete bool
	gap      bool
}

/*
versions is one symbol's book history in venue-time order.

Retention is sized from the data, not a constant: horizon is the largest lag
(newest version time minus requested time) any lookup on the book has asked
for, shared by all symbols because the lag belongs to the pipeline, and every
version needed to answer a lookup that far back is kept, plus the one in
effect at the horizon edge. A lookup older than everything retained is a miss:
it yields no book and is counted, so a horizon still learning shows up as
missing book evidence, never as the live book. limit, from
market.book_history_limit, caps versions per symbol for memory; 0 leaves
retention to the horizon alone.
*/
type versions struct {
	mu      sync.Mutex
	views   []*BookView
	horizon *atomic.Int64
	limit   int
}

func (history *versions) record(view *BookView) {
	history.mu.Lock()
	defer history.mu.Unlock()

	// A version cannot take effect before the one it replaces.
	if count := len(history.views); count > 0 && view.At.Before(history.views[count-1].At) {
		view.At = history.views[count-1].At
	}

	history.views = append(history.views, view)

	// A gap answers nothing itself, so it never pushes the last verified
	// version out: frames stamped before the drop still read that version.
	if !view.gap {
		edge := view.At.Add(-time.Duration(history.horizon.Load()))

		for len(history.views) > 1 && !history.views[1].At.After(edge) {
			history.views[0] = nil
			history.views = history.views[1:]
		}
	}

	for history.limit > 0 && len(history.views) > history.limit {
		history.views[0] = nil
		history.views = history.views[1:]
	}
}

/*
gap marks the book unknown from just after the newest version on, until the
next version (a fresh snapshot) is recorded.
*/
func (history *versions) gap(at time.Time) {
	history.mu.Lock()
	newest := at

	if count := len(history.views); count > 0 && !newest.After(history.views[count-1].At) {
		newest = history.views[count-1].At.Add(time.Nanosecond)
	}

	history.mu.Unlock()
	history.record(&BookView{At: newest, gap: true})
}

/*
lookup returns the version in effect at at: the last one stamped at or before
it. missed reports a lookup older than every retained version.
*/
func (history *versions) lookup(at time.Time) (view *BookView, missed bool) {
	history.mu.Lock()
	defer history.mu.Unlock()

	count := len(history.views)

	if count == 0 {
		return nil, false
	}

	lag := int64(history.views[count-1].At.Sub(at))

	for current := history.horizon.Load(); lag > current; current = history.horizon.Load() {
		if history.horizon.CompareAndSwap(current, lag) {
			break
		}
	}

	index := sort.Search(count, func(i int) bool {
		return history.views[i].At.After(at)
	}) - 1

	if index < 0 {
		return nil, true
	}

	if history.views[index].gap {
		return nil, false
	}

	return history.views[index], false
}

/*
window hands read every verified version in effect during [from, to], oldest
first: the one in effect at from, then each later one stamped at or before to.
Asking for from teaches the shared horizon that lag, exactly as lookup does,
so the next window is retained. covered is false when from is older than
everything retained, so the versions read start later than from.
*/
func (history *versions) window(from, to time.Time, read func(*BookView)) (covered bool) {
	history.mu.Lock()
	defer history.mu.Unlock()

	count := len(history.views)

	if count == 0 {
		return false
	}

	lag := int64(history.views[count-1].At.Sub(from))

	for current := history.horizon.Load(); lag > current; current = history.horizon.Load() {
		if history.horizon.CompareAndSwap(current, lag) {
			break
		}
	}

	first := sort.Search(count, func(i int) bool {
		return history.views[i].At.After(from)
	}) - 1

	covered = first >= 0
	first = max(first, 0)

	for _, view := range history.views[first:] {
		if view.At.After(to) {
			break
		}

		if !view.gap {
			read(view)
		}
	}

	return covered
}

/*
latest returns the newest version, or nil while the book is unknown (no
snapshot yet, or a gap since the last one).
*/
func (history *versions) latest() *BookView {
	history.mu.Lock()
	defer history.mu.Unlock()

	if count := len(history.views); count > 0 && !history.views[count-1].gap {
		return history.views[count-1]
	}

	return nil
}

/*
history returns the symbol's version store, creating it on first use.
*/
func (book *Book) history(symbol string) *versions {
	if val, ok := book.versions.Load(symbol); ok {
		return val.(*versions)
	}

	actual, _ := book.versions.LoadOrStore(symbol, &versions{
		horizon: &book.historyHorizon,
		limit:   viper.GetInt("market.book_history_limit"),
	})

	return actual.(*versions)
}

/*
BookAt hands read the version of symbol's book in effect at venue time at.
It reads nothing when no verified version covers at: before the first
snapshot, after a divergence or stream drop until the next snapshot, or when
at is older than the retained history (counted in HistoryMisses).
*/
func (book *Book) BookAt(symbol string, at time.Time, read func(*BookView)) {
	val, ok := book.versions.Load(symbol)

	if !ok {
		return
	}

	view, missed := val.(*versions).lookup(at)

	if missed {
		book.historyMisses.Add(1)
	}

	if view != nil {
		read(view)
	}
}

/*
BookWindow hands read every verified version of symbol's book in effect during
[from, to]. covered is false when from is older than the retained history.
*/
func (book *Book) BookWindow(symbol string, from, to time.Time, read func(*BookView)) bool {
	val, ok := book.versions.Load(symbol)

	if !ok {
		return false
	}

	return val.(*versions).window(from, to, read)
}

/*
Latest hands read the newest verified version of symbol's book, and reads
nothing while the book is unknown.
*/
func (book *Book) Latest(symbol string, read func(*BookView)) {
	val, ok := book.versions.Load(symbol)

	if !ok {
		return
	}

	if view := val.(*versions).latest(); view != nil {
		read(view)
	}
}

/*
HistoryHorizon is the retention horizon learned from lookups so far.
*/
func (book *Book) HistoryHorizon() time.Duration {
	return time.Duration(book.historyHorizon.Load())
}

/*
HistoryMisses counts lookups that asked for a time older than the retained
history.
*/
func (book *Book) HistoryMisses() int64 {
	return book.historyMisses.Load()
}

/*
captureView copies the verified book's displayed levels. The caller owns the
symbol lock.
*/
func captureView(symbolBook *spotbook.Book, at time.Time) *BookView {
	view := &BookView{
		At:       at,
		FullBid:  len(symbolBook.Bids.Levels) >= symbolBook.MaxDepth,
		FullAsk:  len(symbolBook.Asks.Levels) >= symbolBook.MaxDepth,
		Complete: true,
	}

	for cursor := symbolBook.BestBid(); cursor != nil; cursor = cursor.Lower {
		if cursor.Price == nil || cursor.Quantity == nil {
			view.Complete = false
			continue
		}

		view.Bids = append(view.Bids, BookLevel{
			Price: kraken.Float64(cursor.Price), Quantity: kraken.Float64(cursor.Quantity),
		})
	}

	for cursor := symbolBook.BestAsk(); cursor != nil; cursor = cursor.Higher {
		if cursor.Price == nil || cursor.Quantity == nil {
			view.Complete = false
			continue
		}

		view.Asks = append(view.Asks, BookLevel{
			Price: kraken.Float64(cursor.Price), Quantity: kraken.Float64(cursor.Quantity),
		})
	}

	return view
}

/*
frameTime is the venue time a level3 frame took effect: its own timestamp, or
the latest order timestamp it carries. ok is false for an untimed frame.
*/
func frameTime(data kraken.Level3Data) (at time.Time, ok bool) {
	if !data.Timestamp.IsZero() {
		return data.Timestamp, true
	}

	for _, orders := range [][]kraken.Level3Order{data.Bids, data.Asks} {
		for _, order := range orders {
			if order.Timestamp.After(at) {
				at = order.Timestamp
			}
		}
	}

	return at, !at.IsZero()
}
