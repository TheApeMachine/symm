package hindsight

import (
	"encoding/json"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
)

/*
level3Frame is one captured L3 transport payload (channel "level3") reduced to
the fields hindsight needs to reconstruct an authoritative order book. The raw
payload is decoded once; reconstruction then applies the per-symbol orders in
the exact order the venue delivered them.
*/
type level3Frame struct {
	at   time.Time
	data []kraken.Level3Data
}

/*
BookStore reconstructs one authoritative L3 book per symbol by replaying the
captured level3 snapshots and updates in capture order. It keeps the ordered
events per symbol — not a snapshot of every full book — so counterfactual
execution at a given venue time can rebuild the book as it stood at or before
that time without ever reading future depth.
*/
type BookStore struct {
	symbols map[string][]bookEvent
}

/*
bookEvent is one symbol's L3 data item with the venue time it arrived. It is
retained verbatim so replay applies the same updates the live book manager did.
*/
type bookEvent struct {
	at   time.Time
	data kraken.Level3Data
}

/*
NewBookStore creates an empty L3 reconstruction store.
*/
func NewBookStore() *BookStore {
	return &BookStore{symbols: map[string][]bookEvent{}}
}

/*
Apply ingests one captured level3 payload and appends its per-symbol events in
order. The payload's own "timestamp" is authoritative venue time; when a row
lacks one, the frame arrival time stands in so ordering is never lost.
*/
func (store *BookStore) Apply(payload []byte, frameAt time.Time) error {
	var frame kraken.Level3

	if err := json.Unmarshal(payload, &frame); err != nil {
		return err
	}

	if frame.Channel != "level3" {
		return nil
	}

	for _, data := range frame.Data {
		if data.Symbol == "" {
			continue
		}

		at := data.Timestamp

		if at.IsZero() {
			at = frameAt
		}

		event := bookEvent{at: at, data: data}

		if event.data.Type == "" {
			event.data.Type = frame.Type
		}

		store.symbols[data.Symbol] = append(store.symbols[data.Symbol], event)
	}

	return nil
}

/*
Initialized reports whether the store has received any L3 frame for a symbol.
A symbol with no captured depth has no authoritative book to walk.
*/
func (store *BookStore) Initialized(symbol string) bool {
	events := store.symbols[symbol]
	return len(events) > 0
}

/*
BookAt reconstructs one symbol's authoritative book as it stood at `at`, using
only level3 events whose venue time is at or before `at`. The second result is
false when no snapshot preceded the boundary — the book was uninitialized then,
so executable economics are undefined rather than invented from later depth.
*/
func (store *BookStore) BookAt(symbol string, at time.Time) (*book.Book, bool) {
	cursor := snapshotEventIndex(store.symbols[symbol], at)

	if cursor < 0 {
		return nil, false
	}

	events := store.symbols[symbol]
	reconstructed := book.New()
	reconstructed.NoBookCrossing = false
	reconstructed.EnableMaxDepth = false

	for index := 0; index <= cursor; index++ {
		applyBookEvent(reconstructed, events[index].data)
	}

	return reconstructed, true
}

/*
snapshotEventIndex returns the index of the last snapshot event at or before
`at`, searching backwards from the newest event. A counterfactual boundary
before the first snapshot yields -1: there is no authoritative depth to walk.
*/
func snapshotEventIndex(events []bookEvent, at time.Time) int {
	for index := len(events) - 1; index >= 0; index-- {
		if !events[index].at.After(at) && events[index].data.Type == "snapshot" {
			return index
		}
	}

	return -1
}

/*
applyBookEvent mutates one reconstructed book with one L3 data item, mirroring
the live manager's apply path: a snapshot resets the book, an update carries
add/delete/modify orders applied as level updates.
*/
func applyBookEvent(reconstructed *book.Book, data kraken.Level3Data) {
	if reconstructed == nil {
		return
	}

	if data.Type == "snapshot" {
		resetBook(reconstructed)
	}

	applyOrders(reconstructed, book.Ask, data.Asks)
	applyOrders(reconstructed, book.Bid, data.Bids)
}

func resetBook(reconstructed *book.Book) {
	reconstructed.Bids = book.NewSide()
	reconstructed.Bids.Direction = book.Bid
	reconstructed.Asks = book.NewSide()
	reconstructed.Asks.Direction = book.Ask
}

func applyOrders(
	reconstructed *book.Book,
	direction book.BookDirection,
	orders []kraken.Level3Order,
) {
	for _, order := range orders {
		if order.LimitPrice == nil || order.OrderQty == nil {
			continue
		}

		quantity := order.OrderQty

		if order.Event == "delete" {
			quantity = decimal.NewFromInt64(0).Sub(order.OrderQty)
		}

		reconstructed.Update(&book.UpdateOptions{
			Direction: direction,
			ID:        order.OrderID,
			Price:     order.LimitPrice,
			Quantity:  quantity,
			Timestamp: order.Timestamp,
			Silent:    true,
		})
	}
}

/*
WalkResult is the outcome of walking one side of a reconstructed book for a
requested quantity: the quantity actually filled, the gross value that fill
costs, and the resulting VWAP. A partial fill reports the filled quantity; a
book with no depth at all reports zero.
*/
type WalkResult struct {
	FilledQty float64
	Gross     float64
	VWAP      float64
}

/*
walkAsksImpl and walkBidsImpl are package-level seams so the adversarial
mutation tests can swap in deliberately broken walking (e.g. best-price-only)
and prove the correct-implementation tests catch it. Production code always
uses the honest full-walk implementations below.
*/
var (
	walkAsksImpl func(asks *book.Side, quantity float64) WalkResult = walkAsksHonest
	walkBidsImpl func(bids *book.Side, quantity float64) WalkResult = walkBidsHonest
)

/*
WalkAsks consumes `quantity` from the ask side ascending from the best ask.
*/
func WalkAsks(asks *book.Side, quantity float64) WalkResult {
	return walkAsksImpl(asks, quantity)
}

/*
WalkBids consumes `quantity` from the bid side descending from the best bid.
*/
func WalkBids(bids *book.Side, quantity float64) WalkResult {
	return walkBidsImpl(bids, quantity)
}

func walkAsksHonest(asks *book.Side, quantity float64) WalkResult {
	if asks == nil || quantity <= 0 {
		return WalkResult{}
	}

	remaining := quantity
	gross := 0.0

	for level := asks.Low; level != nil && remaining > 0; level = level.Higher {
		if level.Price == nil || level.Quantity == nil {
			continue
		}

		levelQty := level.Quantity.Float64()

		if levelQty <= 0 {
			continue
		}

		fill := levelQty

		if fill > remaining {
			fill = remaining
		}

		gross += level.Price.Float64() * fill
		remaining -= fill
	}

	filled := quantity - remaining

	if filled <= 0 {
		return WalkResult{}
	}

	return WalkResult{
		FilledQty: filled,
		Gross:     gross,
		VWAP:      gross / filled,
	}
}

func walkBidsHonest(bids *book.Side, quantity float64) WalkResult {
	if bids == nil || quantity <= 0 {
		return WalkResult{}
	}

	remaining := quantity
	gross := 0.0

	for level := bids.High; level != nil && remaining > 0; level = level.Lower {
		if level.Price == nil || level.Quantity == nil {
			continue
		}

		levelQty := level.Quantity.Float64()

		if levelQty <= 0 {
			continue
		}

		fill := levelQty

		if fill > remaining {
			fill = remaining
		}

		gross += level.Price.Float64() * fill
		remaining -= fill
	}

	filled := quantity - remaining

	if filled <= 0 {
		return WalkResult{}
	}

	return WalkResult{
		FilledQty: filled,
		Gross:     gross,
		VWAP:      gross / filled,
	}
}
