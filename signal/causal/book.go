package causal

import (
	"context"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/symm/kraken/market"
)

type Book struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	entity  string
	scope   string
	symbols map[string]*structure.ListRing[*market.BookUpdate]
}

func NewBook(ctx context.Context) *Book {
	ctx, cancel := context.WithCancel(ctx)

	return &Book{
		ctx:     ctx,
		cancel:  cancel,
		entity:  "book",
		symbols: make(map[string]*structure.ListRing[*market.BookUpdate]),
	}
}

func (book *Book) Entity() string {
	return book.entity
}

func (book *Book) Update(update market.BookUpdates) {
	if update == nil {
		return
	}

	for _, bookUpdate := range update {
		if bookUpdate == nil || bookUpdate.Symbol == "" {
			continue
		}

		book.symbols[bookUpdate.Symbol].Push(bookUpdate)
	}
}

func (book *Book) Read(p []byte) (n int, err error) {
	for range book.symbols {
		book.symbols[book.scope].Do(func(update *market.BookUpdate) {
			artifact := datura.Acquire("book", datura.Artifact_Type_json)
			artifact.WithRole("book")
			artifact.WithScope(update.Symbol)
			artifact.WithPayload(update.Marshal())

			n, err = artifact.Read(p)
		})
	}

	return n, err
}
