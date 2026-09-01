package depthflow

import (
	"github.com/theapemachine/symm/kraken"
)

/*
residentBook is the depth-flow signal's own order-by-order book.

Kraken's Level-3 feed is an ORDER feed, not a level feed: every update names an
individual order by OrderID and says whether it was added, changed, or deleted.
The visible notional on a side is therefore a property of the set of live
orders, and the only way to know it is to hold that set.

Summing signed per-message deltas instead cannot reconstruct it:

  - a "change" event carries the order's NEW quantity, so crediting it as a
    positive delta counts the order twice and never removes the quantity it
    replaced;
  - a "delete" event's quantity is not necessarily the resident quantity being
    removed, so subtracting it does not cancel what was added;
  - the opening "snapshot" frame is a full book restatement, not an increment,
    so folding it in as deltas starts the running total from an arbitrary
    point mid-stream.

Each error is one-directional and permanent, so the running sum drifts away
from real depth and — on symbols whose adds and deletes roughly cancel — toward
zero or below it. Everything scaled by depth (reference depth, turnover rate,
and the log-space baseline derived from it) is meaningless once that happens.

The book keeps only what the depth metrics consume — the per-side notional
totals and the touch — so it stays O(live orders) in memory and O(1) per event.
*/
type residentBook struct {
	symbols map[string]*symbolBook
}

type residentOrder struct {
	price float64
	qty   float64
}

/*
undoEntry restores one order slot to what it held before the current frame
touched it. present distinguishes "this order existed with this value" from
"this order did not exist", so a revert removes an order the frame added.
*/
type undoEntry struct {
	orderID string
	bid     bool
	order   residentOrder
	present bool
}

type symbolBook struct {
	bids map[string]residentOrder
	asks map[string]residentOrder

	// journal records each slot's pre-frame value so one frame can be undone
	// without copying the whole book. A frame touches a handful of orders, so
	// the journal stays tiny while the book itself can hold thousands.
	journal      []undoEntry
	priorBid     float64
	priorAsk     float64
	priorSeeded  bool
	journalReset bool
	priorBids    map[string]residentOrder
	priorAsks    map[string]residentOrder

	// bidNotional and askNotional are maintained incrementally against the
	// resident set: each mutation applies the difference between an order's
	// old and new notional, so the totals stay exact without a rescan.
	bidNotional float64
	askNotional float64

	// seeded records whether a snapshot has established the book. Increments
	// arriving before one describe a book this process never saw, so applying
	// them would fabricate depth out of a partial view.
	seeded bool
}

func newResidentBook() *residentBook {
	return &residentBook{symbols: make(map[string]*symbolBook)}
}

func (book *residentBook) symbol(name string) *symbolBook {
	resident, found := book.symbols[name]

	if !found {
		resident = &symbolBook{
			bids: make(map[string]residentOrder),
			asks: make(map[string]residentOrder),
		}
		book.symbols[name] = resident
	}

	return resident
}

/*
usableOrder reports whether an order carries the identity and finite, positive
price/quantity the book needs. A delete is exempt from the quantity check: it
names an order to remove, and the resident quantity is what gets removed.
*/
func usableOrder(order kraken.Level3Order) bool {
	if order.OrderID == "" || order.LimitPrice == nil ||
		order.LimitPrice.Sign() <= 0 {
		return false
	}

	if order.Event == "delete" {
		return true
	}

	return order.OrderQty != nil && order.OrderQty.Sign() > 0
}

/*
apply folds one Level3Data frame into the symbol's resident book and reports
the resulting per-side notional totals and touch.

A "snapshot" frame replaces the book wholesale; every other frame mutates the
resident set. ok is false when the symbol has no seeded book, in which case the
returned depth would be a fiction.

The mutation is not final: the caller runs the metric pipeline against the
resulting state, and a frame the pipeline rejects must leave no trace on the
book. revert undoes exactly this call, restoring the prior order set.
*/
func (book *residentBook) apply(message kraken.Level3Data) (state bookState, ok bool) {
	resident := book.symbol(message.Symbol)

	resident.snapshotPrior()

	if message.Type == "snapshot" {
		resident.reset()

		for _, order := range message.Bids {
			resident.upsert(order, true)
		}

		for _, order := range message.Asks {
			resident.upsert(order, false)
		}

		resident.seeded = true

		return resident.state(), true
	}

	if !resident.seeded {
		return bookState{}, false
	}

	for _, order := range message.Bids {
		resident.applyOrder(order, true)
	}

	for _, order := range message.Asks {
		resident.applyOrder(order, false)
	}

	return resident.state(), true
}

/*
revert rolls the symbol's book back to the state it held before the most recent
apply. The metric pipeline validates a frame AFTER the book has folded it in
(the metrics are computed from the resulting depth), so a frame the pipeline
rejects — a crossed book, say — must not be left resident. Committed state
would otherwise carry the bad orders forward and corrupt every later frame.
*/
func (book *residentBook) revert(symbol string) {
	if resident, found := book.symbols[symbol]; found {
		resident.restorePrior()
	}
}

/*
notionals reports the symbol's per-side notional as it stands BEFORE the next
frame is applied, so a caller can measure that frame's own contribution as a
difference against the resident set. seeded is false when no snapshot has
established the book, in which case there is no prior state to difference
against.
*/
func (book *residentBook) notionals(symbol string) (state bookState, seeded bool) {
	resident, found := book.symbols[symbol]

	if !found || !resident.seeded {
		return bookState{}, false
	}

	return bookState{
		bidNotional: resident.bidNotional,
		askNotional: resident.askNotional,
	}, true
}

/*
snapshotPrior opens an undo scope for one frame: the journal is emptied and the
side totals recorded, so every mutation the frame makes can be rolled back.
*/
func (resident *symbolBook) snapshotPrior() {
	resident.journal = resident.journal[:0]
	resident.journalReset = false
	resident.priorBid = resident.bidNotional
	resident.priorAsk = resident.askNotional
	resident.priorSeeded = resident.seeded
	resident.priorBids = nil
	resident.priorAsks = nil
}

/*
restorePrior rolls back every slot the current frame touched, in reverse order
so a slot written more than once lands on its original value.
*/
func (resident *symbolBook) restorePrior() {
	// A snapshot replaced the whole book, so the journal cannot express the
	// rollback; the full prior maps were retained instead.
	if resident.journalReset {
		resident.bids = resident.priorBids
		resident.asks = resident.priorAsks
		resident.priorBids = nil
		resident.priorAsks = nil
	} else {
		for index := len(resident.journal) - 1; index >= 0; index-- {
			entry := resident.journal[index]
			side := resident.asks

			if entry.bid {
				side = resident.bids
			}

			if entry.present {
				side[entry.orderID] = entry.order

				continue
			}

			delete(side, entry.orderID)
		}
	}

	resident.journal = resident.journal[:0]
	resident.journalReset = false
	resident.bidNotional = resident.priorBid
	resident.askNotional = resident.priorAsk
	resident.seeded = resident.priorSeeded
}

/*
record journals one order slot's pre-mutation value. It must be called before
the slot is written.
*/
func (resident *symbolBook) record(orderID string, bid bool) {
	side := resident.asks

	if bid {
		side = resident.bids
	}

	order, present := side[orderID]
	resident.journal = append(resident.journal, undoEntry{
		orderID: orderID,
		bid:     bid,
		order:   order,
		present: present,
	})
}

func (resident *symbolBook) reset() {
	// A snapshot discards the entire book, which no per-slot journal can
	// undo, so the prior maps are retained wholesale for the rollback.
	if !resident.journalReset {
		resident.journalReset = true
		resident.priorBids = resident.bids
		resident.priorAsks = resident.asks
		resident.bids = make(map[string]residentOrder, len(resident.bids))
		resident.asks = make(map[string]residentOrder, len(resident.asks))
		resident.bidNotional = 0
		resident.askNotional = 0

		return
	}

	resident.clear()
}

func (resident *symbolBook) clear() {
	clear(resident.bids)
	clear(resident.asks)
	resident.bidNotional = 0
	resident.askNotional = 0
}

func (resident *symbolBook) applyOrder(order kraken.Level3Order, bid bool) {
	if !usableOrder(order) {
		return
	}

	if order.Event == "delete" {
		resident.remove(order.OrderID, bid)

		return
	}

	resident.upsert(order, bid)
}

/*
upsert installs an order under its identity, crediting only the DIFFERENCE
between its previous resident notional and its new one. This is what makes a
"change" event correct: the quantity it carries replaces the order's quantity
rather than adding to it.
*/
func (resident *symbolBook) upsert(order kraken.Level3Order, bid bool) {
	if !usableOrder(order) || order.OrderQty == nil {
		return
	}

	side := resident.asks

	if bid {
		side = resident.bids
	}

	price, qty := order.LimitPrice.Float64(), order.OrderQty.Float64()
	next := residentOrder{price: price, qty: qty}
	delta := next.notional()

	if previous, found := side[order.OrderID]; found {
		delta -= previous.notional()
	}

	resident.record(order.OrderID, bid)
	side[order.OrderID] = next
	resident.credit(delta, bid)
}

/*
remove withdraws the order's OWN resident notional, which is the quantity that
was actually contributing to depth — not whatever quantity the delete message
happened to carry.
*/
func (resident *symbolBook) remove(orderID string, bid bool) {
	side := resident.asks

	if bid {
		side = resident.bids
	}

	previous, found := side[orderID]

	if !found {
		return
	}

	resident.record(orderID, bid)
	delete(side, orderID)
	resident.credit(-previous.notional(), bid)
}

func (resident *symbolBook) credit(delta float64, bid bool) {
	if bid {
		resident.bidNotional += delta

		return
	}

	resident.askNotional += delta
}

func (order residentOrder) notional() float64 { return order.price * order.qty }

/*
bookState is one frame's view of the resident book: the per-side visible
notional and the touch, each derived from the live order set rather than from
whatever a single message happened to mention.
*/
type bookState struct {
	bidNotional float64
	askNotional float64

	touchBidPrice    float64
	touchAskPrice    float64
	touchBidNotional float64
	touchAskNotional float64
}

/*
state derives the touch from the resident set. The touch is the best live
price on each side and the notional resting at exactly that price — summed
across every order there, since price levels hold many orders.
*/
func (resident *symbolBook) state() bookState {
	state := bookState{
		bidNotional: resident.bidNotional,
		askNotional: resident.askNotional,
	}

	for _, order := range resident.bids {
		if order.price > state.touchBidPrice {
			state.touchBidPrice = order.price
		}
	}

	for _, order := range resident.asks {
		if state.touchAskPrice == 0 || order.price < state.touchAskPrice {
			state.touchAskPrice = order.price
		}
	}

	for _, order := range resident.bids {
		if order.price == state.touchBidPrice {
			state.touchBidNotional += order.notional()
		}
	}

	for _, order := range resident.asks {
		if order.price == state.touchAskPrice {
			state.touchAskNotional += order.notional()
		}
	}

	return state
}
