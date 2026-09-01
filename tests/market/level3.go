/*
Package market builds realistic market data tapes for tests.

A hand-made fixture only ever contains the case its author was thinking about.
Every Level-3 consumer in this repo was tested against fixtures whose helper
hardcoded Event: "add" and supplied both sides of the book in one message, so
whole classes of real wire traffic — deletes, one-sided openings, a fresh price
transiently crossing the retained opposite side — were structurally unreachable
by the tests and shipped broken.

This package emits what Kraken's Level-3 feed actually sends, so a consumer is
exercised against the wire it will really see rather than the wire its test
author imagined.
*/
package market

import (
	"math/rand"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
)

/*
Level3Tape is a deterministic Level-3 message sequence for one symbol. It owns
the true book it is describing, so a test can assert a consumer's derived touch
against the touch that actually exists rather than against a restatement of the
consumer's own arithmetic.
*/
type Level3Tape struct {
	Symbol   string
	Messages []kraken.Level3Data

	// TrueBid and TrueAsk are the real touch after each message, index-aligned
	// with Messages. A side is zero until the tape has established it.
	TrueBid []float64
	TrueAsk []float64
}

/*
level3Book is the resting book the tape describes, keyed by order id so a
delete removes the identity it names rather than a price level.
*/
type level3Book struct {
	bids map[string]float64
	asks map[string]float64
	qty  map[string]float64
}

func newLevel3Book() *level3Book {
	return &level3Book{
		bids: map[string]float64{},
		asks: map[string]float64{},
		qty:  map[string]float64{},
	}
}

func (book *level3Book) touch() (bid, ask float64) {
	for _, price := range book.bids {
		if price > bid {
			bid = price
		}
	}

	for _, price := range book.asks {
		if ask == 0 || price < ask {
			ask = price
		}
	}

	return bid, ask
}

func order(event, id string, price, quantity float64, at time.Time) kraken.Level3Order {
	return kraken.Level3Order{
		Event:      event,
		OrderID:    id,
		LimitPrice: decimal.NewFromFloat64(price),
		OrderQty:   decimal.NewFromFloat64(quantity),
		Timestamp:  at,
	}
}

/*
append records one message and the true touch that holds after it.
*/
func (tape *Level3Tape) append(book *level3Book, at time.Time, bids, asks []kraken.Level3Order) {
	tape.Messages = append(tape.Messages, kraken.Level3Data{
		Symbol:    tape.Symbol,
		Timestamp: at,
		Bids:      bids,
		Asks:      asks,
	})

	bid, ask := book.touch()
	tape.TrueBid = append(tape.TrueBid, bid)
	tape.TrueAsk = append(tape.TrueAsk, ask)
}

/*
addBid places a resting bid and emits the one-sided message announcing it.
*/
func (tape *Level3Tape) addBid(book *level3Book, id string, price, quantity float64, at time.Time) {
	book.bids[id] = price
	book.qty[id] = quantity
	tape.append(book, at, []kraken.Level3Order{order("add", id, price, quantity, at)}, nil)
}

/*
addAsk places a resting ask and emits the one-sided message announcing it.
*/
func (tape *Level3Tape) addAsk(book *level3Book, id string, price, quantity float64, at time.Time) {
	book.asks[id] = price
	book.qty[id] = quantity
	tape.append(book, at, nil, []kraken.Level3Order{order("add", id, price, quantity, at)})
}

/*
deleteBid removes a resting bid. The wire still carries the order's price and
quantity, which is exactly why a consumer that ignores Event reads removed
liquidity as resting.
*/
func (tape *Level3Tape) deleteBid(book *level3Book, id string, at time.Time) {
	price, quantity := book.bids[id], book.qty[id]
	delete(book.bids, id)
	tape.append(book, at, []kraken.Level3Order{order("delete", id, price, quantity, at)}, nil)
}

/*
deleteAsk removes a resting ask.
*/
func (tape *Level3Tape) deleteAsk(book *level3Book, id string, at time.Time) {
	price, quantity := book.asks[id], book.qty[id]
	delete(book.asks, id)
	tape.append(book, at, nil, []kraken.Level3Order{order("delete", id, price, quantity, at)})
}

/*
NewLevel3Tape builds a tape covering the Level-3 traffic every consumer must
survive. It deliberately opens ONE-SIDED, mutates through deletes, and walks
the touch down so a fresh price on one side transiently sits through the
other side's last known price — the three shapes that hand-made both-sides
add-only fixtures cannot express.

Depth is quoted as a real book: several resting orders per side, so a delete of
the best order exposes the next level rather than emptying the side.
*/
func NewLevel3Tape(symbol string, start time.Time) *Level3Tape {
	tape := &Level3Tape{Symbol: symbol}
	book := newLevel3Book()
	at := start
	step := func() time.Time {
		at = at.Add(10 * time.Millisecond)

		return at
	}

	// The feed opens one side at a time: no consumer may assume a first
	// message carries a complete touch.
	tape.addBid(book, "b1", 99.0, 10, step())
	tape.addBid(book, "b2", 98.5, 8, step())
	tape.addAsk(book, "a1", 101.0, 12, step())
	tape.addAsk(book, "a2", 101.5, 6, step())

	// Ordinary two-sided churn once the book is established.
	tape.addBid(book, "b3", 99.5, 4, step())
	tape.addAsk(book, "a3", 100.5, 5, step())

	// A delete of the best bid: the wire carries its price and quantity, but
	// the true best bid falls back to the next resting order.
	tape.deleteBid(book, "b3", step())

	// The market falls. The bids that would be crossed are withdrawn FIRST —
	// a real book is never crossed — and only then does the ask side quote
	// down through where those bids used to be.
	//
	// This is the shape that breaks a naive consumer: each of these messages
	// is one-sided, so a consumer merging this message's ask against the LAST
	// BID IT SAW (99.0, now withdrawn) sees ask 98.0 below bid 99.0 and calls
	// the book crossed. The true book never is.
	tape.deleteBid(book, "b1", step())
	tape.deleteBid(book, "b2", step())
	tape.addAsk(book, "a4", 98.0, 7, step())
	tape.addBid(book, "b4", 97.0, 9, step())

	return tape
}

/*
NewLevel3DeleteOnlyTape is a book that is entirely withdrawn. A consumer that
treats a delete as resting liquidity reports a full book here; a correct one
reports no touch at all after the removals.
*/
func NewLevel3DeleteOnlyTape(symbol string, start time.Time) *Level3Tape {
	tape := &Level3Tape{Symbol: symbol}
	book := newLevel3Book()
	at := start
	step := func() time.Time {
		at = at.Add(10 * time.Millisecond)

		return at
	}

	tape.addBid(book, "b1", 50.0, 3, step())
	tape.addAsk(book, "a1", 51.0, 3, step())
	tape.deleteBid(book, "b1", step())
	tape.deleteAsk(book, "a1", step())

	return tape
}

/*
NewLevel3ChurnTape is a longer deterministic tape mixing adds and deletes
across many price levels, for consumers whose estimators need depth before
they report. The sequence is seeded, so it is identical on every run.
*/
func NewLevel3ChurnTape(symbol string, start time.Time, messages int) *Level3Tape {
	tape := &Level3Tape{Symbol: symbol}
	book := newLevel3Book()
	rng := rand.New(rand.NewSource(42))
	at := start
	live := []string{}
	next := 0

	tape.addBid(book, "seed-b", 99.0, 10, at.Add(10*time.Millisecond))
	at = at.Add(10 * time.Millisecond)
	tape.addAsk(book, "seed-a", 101.0, 10, at.Add(10*time.Millisecond))
	at = at.Add(10 * time.Millisecond)

	for index := 0; index < messages; index++ {
		at = at.Add(time.Duration(1+rng.Intn(50)) * time.Millisecond)

		// Withdraw an existing order roughly a third of the time, so deletes
		// are a normal part of the tape rather than an edge case.
		if len(live) > 0 && rng.Intn(3) == 0 {
			victim := live[rng.Intn(len(live))]
			live = removeID(live, victim)

			if _, isBid := book.bids[victim]; isBid {
				tape.deleteBid(book, victim, at)

				continue
			}

			tape.deleteAsk(book, victim, at)

			continue
		}

		next++
		id := "o" + itoa(next)
		live = append(live, id)

		if rng.Intn(2) == 0 {
			tape.addBid(book, id, 98.0+rng.Float64(), 1+rng.Float64()*9, at)

			continue
		}

		tape.addAsk(book, id, 101.0+rng.Float64(), 1+rng.Float64()*9, at)
	}

	return tape
}

func removeID(ids []string, target string) []string {
	for index, id := range ids {
		if id == target {
			return append(ids[:index], ids[index+1:]...)
		}
	}

	return ids
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}

	digits := []byte{}

	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}

	return string(digits)
}
