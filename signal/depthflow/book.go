package depthflow

import (
	"context"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
)

/*
Book ingests public book rows into the shared depth-flow sample.
*/
type Book struct {
	ctx    context.Context
	cancel context.CancelFunc
	api    *websocket.API
	cache  []kraken.BookData
}

func NewBook(ctx context.Context, api *websocket.API) *Book {
	ctx, cancel := context.WithCancel(ctx)

	book := &Book{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
		cache:  []kraken.BookData{},
	}

	book.api.On("book", book.On)
	return book
}

func (book *Book) On(data []byte) {
	if len(data) == 0 {
		return
	}

	frame := kraken.NewBook(data)

	if len(frame.Data) == 0 {
		return
	}

	book.cache = append(book.cache, frame.Data...)
}
