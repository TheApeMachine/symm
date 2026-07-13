package toxicity

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/kraken/websocket"
)

/*
Level3 reads the SDK's managed level3 order books for the shared toxicity
sample. The BookManager applies and checksums every snapshot and delta, so
Level3 does not decode raw frames itself.
*/
type Level3 struct {
	ctx    context.Context
	cancel context.CancelFunc
	api    *websocket.API
}

func NewLevel3(ctx context.Context, api *websocket.API) *Level3 {
	ctx, cancel := context.WithCancel(ctx)

	return &Level3{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
	}
}

/*
Books returns every order book the SDK currently manages for this transport.
*/
func (level3 *Level3) Books() []*book.Book {
	manager := level3.api.Books()
	symbols := manager.GetBooks()
	books := make([]*book.Book, 0, len(symbols))

	for _, symbol := range symbols {
		books = append(books, manager.GetBook(symbol))
	}

	return books
}
