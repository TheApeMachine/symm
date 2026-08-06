package kraken

import (
	"context"

	spotbook "github.com/theapemachine/api-go/v2/pkg/book"
	"github.com/theapemachine/api-go/v2/pkg/callback"
	"github.com/theapemachine/api-go/v2/pkg/kraken"
	"github.com/theapemachine/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

type Websocket struct {
	ctx         context.Context
	cancel      context.CancelFunc
	status      types.Status
	client      *spot.WebSocket
	bookManager *spot.BookManager
}

func NewWebsocket(ctx context.Context) *Websocket {
	ctx, cancel := context.WithCancel(ctx)

	websocket := &Websocket{
		ctx:         ctx,
		cancel:      cancel,
		status:      types.INITIALIZING,
		client:      spot.NewWebSocket(),
		bookManager: spot.NewBookManager(),
	}

	websocket.bookManager.OnCreateBook.Recurring(func(
		event *callback.Event[*spotbook.Book],
	) {
		errnie.Info("book created:", event.Data.Name)

		book := event.Data

		book.OnUpdated.Recurring(func(e *callback.Event[*spotbook.UpdateOptions]) {
			// fmt.Printf("%s: %s\n", b.Name, helper.ToJSON(e.Data))
		})

		book.OnBookCrossed.Recurring(func(event *callback.Event[*spotbook.CrossedResult]) {
		})

		book.OnMaxDepthExceeded.Recurring(func(event *callback.Event[*spotbook.MaxDepthExceededResult]) {
		})

		book.OnChecksummed.Recurring(func(event *callback.Event[*spotbook.ChecksumResult]) {
			if !event.Data.Match {
			}
		})
	})

	websocket.client.OnSent.Recurring(func(event *callback.Event[*kraken.WebSocketMessage]) {
		if err := websocket.bookManager.Update(event); err != nil {
			panic(err)
		}
	})

	websocket.client.OnReceived.Recurring(func(event *callback.Event[*kraken.WebSocketMessage]) {
		if err := websocket.bookManager.Update(event); err != nil {
			panic(err)
		}
	})

	websocket.client.OnAuthenticated.Recurring(func(e *callback.Event[string]) {
		if err := websocket.client.SubL3([]string{"BTC/USD"}, 10); err != nil {
			panic(err)
		}
	})

	websocket.client.OnConnected.Recurring(func(e *callback.Event[any]) {
		if err := websocket.client.Authenticate(); err != nil {
			panic(err)
		}
	})

	return websocket
}
