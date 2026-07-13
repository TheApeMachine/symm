package websocket

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

const (
	PublicWebSocketURL  = "wss://ws.kraken.com/v2"
	PrivateWebSocketURL = "wss://ws-auth.kraken.com/v2"
	Level3WebSocketURL  = "wss://ws-l3.kraken.com/v2"
)

/*
Live is the spot websocket and REST transport.
*/
type Live struct {
	status    types.Status
	ctx       context.Context
	cancel    context.CancelFunc
	client    *spot.WebSocket
	sync      *sync.Map
	paper     *Paper
	simulator *Simulator
	auth      bool
	books     *spot.BookManager
	isLevel3  bool
}

/*
New opens a spot websocket transport.
*/
func New(
	ctx context.Context,
	simulator *Simulator,
	auth bool,
	endpoint string,
) *Live {
	ctx, cancel := context.WithCancel(ctx)

	live := &Live{
		status:    types.INITIALIZING,
		ctx:       ctx,
		cancel:    cancel,
		simulator: simulator,
		client:    spot.NewWebSocket(),
		sync:      &sync.Map{},
		auth:      auth,
	}

	live.client.URL = endpoint

	if endpoint == Level3WebSocketURL {
		live.isLevel3 = true
	}

	if live.auth {
		live.client.REST.PublicKey = os.Getenv("KRAKEN_API_KEY")
		live.client.REST.PrivateKey = os.Getenv("KRAKEN_API_SECRET")

		// Kraken remembers the last accepted nonce per key across process
		// restarts. The vendored counter defaults to second granularity, so
		// any restart landing within the same wall-clock second as the prior
		// run's last request resets to a lower nonce and gets rejected.
		// Microsecond granularity keeps the restart-collision window well
		// below realistic process startup latency while staying inside the
		// int64 range Kraken expects for the nonce field.
		nonceCounter := kraken.NewEpochCounter()
		nonceCounter.Granularity = time.Microsecond
		live.client.REST.Nonce = nonceCounter.Get
	}

	if live.isLevel3 {
		live.books = spot.NewBookManager()
		live.books.OnCreateBook.Recurring(func(e *callback.Event[*book.Book]) {
			manager := e.Data

			manager.OnUpdated.Recurring(func(e *callback.Event[*book.UpdateOptions]) {
			})

			manager.OnBookCrossed.Recurring(func(e *callback.Event[*book.CrossedResult]) {
			})

			manager.OnMaxDepthExceeded.Recurring(func(e *callback.Event[*book.MaxDepthExceededResult]) {
			})

			manager.OnChecksummed.Recurring(func(e *callback.Event[*book.ChecksumResult]) {
				if !e.Data.Match {
				}
			})
		})
	}

	live.client.OnSent.Recurring(func(event *callback.Event[*kraken.WebSocketMessage]) {
		if live.isLevel3 {
			if err := live.books.Update(event); err != nil {
				errnie.Error(errnie.Err(errnie.Validation, err.Error(), err))
			}
		}
	})

	live.client.OnReceived.Recurring(func(event *callback.Event[*kraken.WebSocketMessage]) {
		if live.isLevel3 {
			if err := live.books.Update(event); err != nil {
				errnie.Error(errnie.Err(errnie.Validation, err.Error(), err))
			}
		}

		live.route(event.Data.Bytes())
	})

	live.client.OnConnected.Recurring(func(event *callback.Event[any]) {
		if !live.auth {
			live.status = types.READY
			return
		}

		if errnie.Error(live.client.Authenticate()) != nil {
			live.status = types.ERROR
		}
	})

	if live.auth {
		live.client.OnAuthenticated.Recurring(func(event *callback.Event[string]) {
			live.status = types.READY
		})
	}

	return live
}

func (live *Live) Initialize() error {
	errnie.Info("initializing live")

	if err := live.client.Connect(); err != nil {
		live.status = types.ERROR
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: connect failed",
			err,
		))
	}

	live.status = types.READY
	return nil
}

func (live *Live) route(raw []byte) {
	channel := utils.GetString(raw, "channel")

	if channel == "" {
		if message := utils.GetString(raw, "error"); message != "" {
			errnie.Error(errnie.Err(errnie.Validation, message, nil))
		}

		return
	}

	if channel == "status" || channel == "heartbeat" {
		return
	}

	callbacks, ok := live.sync.Load(channel)

	if !ok {
		if channel != "" {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"websocket: channel "+channel+" not found",
				nil,
			))
		}

		return
	}

	for _, callback := range callbacks.([]func([]byte)) {
		callback(raw)
	}
}

func (live *Live) Status() types.Status {
	return live.status
}

func (live *Live) Client() *spot.WebSocket {
	return live.client
}

/*
Books returns the managed SDK order books for this transport, if any.
Only the level3 transport maintains book state.
*/
func (live *Live) Books() *spot.BookManager {
	return live.books
}

func (live *Live) On(
	channel string, action func([]byte),
) {
	callbacks, ok := live.sync.LoadOrStore(channel, []func([]byte){action})

	if ok {
		callbacks = append(callbacks.([]func([]byte)), action)
		live.sync.Store(channel, callbacks)
	}
}

func (live *Live) Write(params json.Marshaler) error {
	raw, err := params.MarshalJSON()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: write marshal failed",
			err,
		))
	}

	methodNode, err := sonic.Get(raw, "method")

	if err != nil || !methodNode.Exists() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}

	started := time.Now()

	writeErr := live.client.WriteMessage(
		gorillawebsocket.TextMessage, raw,
	)

	if live.simulator != nil {
		live.simulator.Record(WEBSOCKET, time.Since(started))
	}

	return errnie.Error(writeErr)
}

func (live *Live) do(options spot.RequestOptions) ([]byte, error) {
	started := time.Now()

	request, err := live.client.REST.NewRequest(options)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}

	resp, err := request.Do()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}

	if live.simulator != nil {
		live.simulator.Record(REST, time.Since(started))
	}

	return resp.Body, nil
}

func (live *Live) Get(
	path string, params json.Marshaler,
) ([]byte, error) {
	return live.do(spot.RequestOptions{
		Auth:   live.auth,
		Path:   path,
		Method: "GET",
		Query:  params,
	})
}

func (live *Live) Post(
	path string, params json.Marshaler,
) ([]byte, error) {
	return live.do(spot.RequestOptions{
		Auth:   live.auth,
		Path:   path,
		Method: "POST",
		Body:   params,
	})
}

func (live *Live) Close() {
	if live.paper != nil {
		live.paper.Close()
	}

	live.cancel()
	live.client.Disconnect()
}
