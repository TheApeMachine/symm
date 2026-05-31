package market

import (
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/orderbook"
)

/*
BookFeedState is the shared maintained-book + checksum gate used by book-driven
signals. It centralizes snapshot ordering and divergence handling so replay and
live feeds behave the same way.
*/
type BookFeedState struct {
	book     *orderbook.Book
	sequence BookSequence
	diverged bool
	symbol   string
	signal   string
}

func NewBookFeedState(symbol string, signal string, depth int) *BookFeedState {
	return &BookFeedState{
		symbol: symbol,
		signal: signal,
		book:   orderbook.NewBook(orderbook.MaintainDepth(depth)),
	}
}

func (state *BookFeedState) Ready() bool {
	return state.book.Ready() && !state.diverged
}

func (state *BookFeedState) Book() *orderbook.Book {
	return state.book
}

func (state *BookFeedState) Diverged() bool {
	return state.diverged
}

func (state *BookFeedState) Accepts(update BookUpdate) bool {
	return state.sequence.Accepts(update)
}

func (state *BookFeedState) Apply(update BookUpdate) {
	if !state.sequence.Accepts(update) {
		return
	}

	if update.IsSnapshot() {
		state.book.ApplySnapshot(update.BidLevels(), update.AskLevels())
		state.verify(uint32(update.Checksum))

		return
	}

	state.book.ApplyDelta(update.BidLevels(), update.AskLevels())
	state.verify(uint32(update.Checksum))
}

func (state *BookFeedState) verify(checksum uint32) {
	if checksum == 0 || !state.book.Ready() {
		return
	}

	matches := state.book.Verify(checksum)

	if !matches && !state.diverged {
		errnie.Error(fmt.Errorf("%s: book checksum diverged for %s", state.signal, state.symbol))
	}

	state.diverged = !matches
}
