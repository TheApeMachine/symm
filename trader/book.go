package trader

import (
	"context"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/symm/kraken/market"
)

type Book struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	books  structure.Ring[market.BookUpdate]
}

func NewBook(ctx context.Context) *Book {
	ctx, cancel := context.WithCancel(ctx)

	book := &Book{
		ctx:    ctx,
		cancel: cancel,
	}

	return book
}

func (book *Book) Update(update market.BookUpdates) {
	if book.books == nil {
		book.books = structure.NewListRing[market.BookUpdate](
			len(update),
			datura.Acquire("book", datura.Artifact_Type_json),
		)
	}
}

func (book *Book) Error() error {
	return book.err
}

func (book *Book) Close() error {
	book.cancel()
	return nil
}
