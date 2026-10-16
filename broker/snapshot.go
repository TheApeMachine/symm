package broker

import (
	"github.com/bytedance/sonic"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/data"
)

/*
Level is one price level of a stamped book side.
*/
type Level struct {
	Price    *decimal.Decimal `json:"price"`
	Quantity *decimal.Decimal `json:"quantity"`
}

/*
Snapshot is a symbol's resident book as ingress stamped it onto an event: every
level of both sides inside the book's depth window, best first, and the window
size itself. It is taken under the symbol's read lock at the event's sequence
number, so it is exactly the book a SeqIdx-ordered replay of the stored level3
tape rebuilds at that event. A snapshot always holds an executable touch: an
unseeded, diverged, missing, one-sided, or crossed book is never stamped.

Every consumer of book state reads the snapshot its event carries, never the
live book, so a consumer that runs later than ingress still sees the book the
event happened against.
*/
type Snapshot struct {
	Depth int     `json:"depth"`
	Bids  []Level `json:"bids"`
	Asks  []Level `json:"asks"`
}

/*
Touch returns the snapshot's best bid and best ask.
*/
func (snapshot *Snapshot) Touch() (bid Level, ask Level) {
	return snapshot.Bids[0], snapshot.Asks[0]
}

/*
Full reports whether a side fills the depth window, so prices beyond its worst
level may rest outside the window rather than be absent from the book.
*/
func (snapshot *Snapshot) Full(levels []Level) bool {
	return len(levels) == snapshot.Depth
}

/*
Equal reports whether two snapshots hold the same window and the same levels to
the venue's exact digits.
*/
func (snapshot *Snapshot) Equal(other *Snapshot) bool {
	if snapshot == nil || other == nil {
		return snapshot == other
	}

	if snapshot.Depth != other.Depth {
		return false
	}

	return sameLevels(snapshot.Bids, other.Bids) && sameLevels(snapshot.Asks, other.Asks)
}

func sameLevels(left, right []Level) bool {
	if len(left) != len(right) {
		return false
	}

	for index := range left {
		if left[index].Price.Cmp(right[index].Price) != 0 {
			return false
		}

		if left[index].Quantity.Cmp(right[index].Quantity) != 0 {
			return false
		}
	}

	return true
}

/*
ReadSnapshot decodes the snapshot stamped onto value.
*/
func ReadSnapshot(value *data.Value) (*Snapshot, error) {
	if value == nil || len(value.Layers) == 0 {
		return nil, nil
	}

	for layer := range data.Layers[[]byte](value, "snapshot") {

		var snap Snapshot
		if err := sonic.Unmarshal(layer, &snap); err == nil && len(snap.Bids) > 0 && len(snap.Asks) > 0 {
			return &snap, nil
		}
	}

	for layer := range data.Layers[[]byte](value, "book") {
		var snap Snapshot
		if err := sonic.Unmarshal(layer, &snap); err == nil && len(snap.Bids) > 0 && len(snap.Asks) > 0 {
			return &snap, nil
		}

		var b spotbook.Book
		if err := sonic.Unmarshal(layer, &b); err == nil && b.BestBid() != nil && b.BestAsk() != nil {
			var bids, asks []Level
			for bid := b.BestBid(); bid != nil; bid = bid.Lower {
				bids = append(bids, Level{Price: bid.Price, Quantity: bid.Quantity})
			}
			for ask := b.BestAsk(); ask != nil; ask = ask.Higher {
				asks = append(asks, Level{Price: ask.Price, Quantity: ask.Quantity})
			}
			if len(bids) > 0 && len(asks) > 0 {
				return &Snapshot{
					Depth: len(bids),
					Bids:  bids,
					Asks:  asks,
				}, nil
			}
		}
	}

	return nil, nil
}

