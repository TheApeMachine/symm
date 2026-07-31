package websocket

import (
	"context"
	"fmt"
	"sync"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

type Book struct {
	ctx         context.Context
	cancel      context.CancelFunc
	status      types.Status
	manager     *spot.BookManager
	subscribers *sync.Map
}

func NewBook(ctx context.Context) *Book {
	errnie.Info("websocket: initializing book manager")
	ctx, cancel := context.WithCancel(ctx)

	book := &Book{
		ctx:         ctx,
		cancel:      cancel,
		status:      types.INITIALIZING,
		manager:     spot.NewBookManager(),
		subscribers: &sync.Map{},
	}

	book.manager.OnCreateBook.Recurring(func(
		event *callback.Event[*spotbook.Book],
	) {
		errnie.Info(fmt.Sprintf("websocket: new book created for %s", event.Data.Name))
		managed := event.Data

		if managed == nil {
			return
		}

		managed.EnableMaxDepth = false
		managed.NoBookCrossing = false
		book.status = types.READY

		managed.OnUpdated.Recurring(func(
			bookEvent *callback.Event[*spotbook.UpdateOptions],
		) {
			book.subscribers.Range(func(key, value any) bool {
				subscriber, ok := value.(types.Subscription[*spotbook.Book])

				if ok {
					subscriber.Send(managed)
				}

				return true
			})
		})

		managed.OnBookCrossed.Recurring(func(
			bookEvent *callback.Event[*spotbook.CrossedResult],
		) {
			managed.EnforceOrder()
		})

		managed.OnMaxDepthExceeded.Recurring(func(
			bookEvent *callback.Event[*spotbook.MaxDepthExceededResult],
		) {
			managed.EnforceDepth()
		})

		managed.OnChecksummed.Recurring(func(
			bookEvent *callback.Event[*spotbook.ChecksumResult],
		) {
			if !bookEvent.Data.Match {
				book.status = types.ERROR

				errnie.Error(errnie.Err(
					errnie.Validation,
					fmt.Sprintf(
						"checksum mismatch for local: %s, and server: %s",
						bookEvent.Data.LocalChecksum,
						bookEvent.Data.ServerChecksum,
					),
					nil,
				))
			}
		})
	})

	return book
}

func (book *Book) Status() types.Status {
	return book.status
}

func (book *Book) Get(symbol string) *spotbook.Book {
	return book.manager.GetBook(symbol)
}

func (book *Book) All() map[string]*spotbook.Book {
	out := make(map[string]*spotbook.Book)

	for _, symbol := range book.manager.GetBooks() {
		out[symbol] = book.manager.GetBook(symbol)
	}

	return out
}
