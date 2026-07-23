package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	gorillawebsocket "github.com/gorilla/websocket"
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
Live is the spot websocket and REST transport. Authentication, reconnect
replay, and callback wiring are owned by the composed Transport.
*/
type Live struct {
	status       atomic.Value
	ctx          context.Context
	cancel       context.CancelFunc
	client       *spot.WebSocket
	sync         *sync.Map
	handlerMu    sync.Mutex
	nextID       atomic.Uint64
	paper        *Paper
	simulator    *Simulator
	books        *spot.BookManager
	bookMu       sync.RWMutex
	isLevel3     bool
	symbols      []string
	transport    *Transport
	level3Queue  chan []byte
	level3Ledger *level3Ledger
}

/*
New opens a spot websocket transport and delegates lifecycle wiring to
Transport so connect, auth, and replay stay out of the construction path.
*/
func New(
	ctx context.Context,
	simulator *Simulator,
	auth bool,
	endpoint string,
) *Live {
	ctx, cancel := context.WithCancel(ctx)
	transport := NewTransport(auth)

	live := &Live{
		ctx:       ctx,
		cancel:    cancel,
		simulator: simulator,
		client:    spot.NewWebSocket(),
		sync:      &sync.Map{},
		transport: transport,
	}
	live.status.Store(types.INITIALIZING)
	live.client.URL = endpoint

	if endpoint == Level3WebSocketURL {
		live.isLevel3 = true
		configureLevel3(live)
	}

	transport.wireCredentials(live.client)
	transport.bindCallbacks(live)

	return live
}

/*
OnReconnect registers a reconnect replay callback on the composed Transport.
*/
func (live *Live) OnReconnect(fn func() error) {
	if live == nil || live.transport == nil {
		return
	}

	live.transport.OnReconnect(fn)
}

func (live *Live) Initialize() error {
	errnie.Info("initializing live")

	if err := live.client.Connect(); err != nil {
		live.status.Store(types.ERROR)

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: connect failed",
			err,
		))
	}

	return nil
}

func (live *Live) route(raw []byte) {
	channel := utils.GetString(raw, "channel")

	if channel == "" {
		if method := utils.GetString(raw, "method"); method == "add_order" {
			channel = method
		}
	}

	if channel == "" {
		if message := utils.GetString(raw, "error"); message != "" {
			errnie.Error(errnie.Err(errnie.Validation, message, nil))
		}

		return
	}

	if channel == "status" || channel == "heartbeat" {
		return
	}

	if live.isLevel3 && channel == "level3" {
		return
	}

	live.dispatch(channel, raw)
}

func (live *Live) dispatch(channel string, raw []byte) {
	live.handlerMu.Lock()
	callbacks, ok := live.sync.Load(channel)

	if !ok {
		live.handlerMu.Unlock()
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: channel "+channel+" not found",
			nil,
		))

		return
	}

	handlers := append([]channelHandler(nil), callbacks.([]channelHandler)...)
	live.handlerMu.Unlock()

	for _, handler := range handlers {
		handler.fn(raw)
	}
}

func (live *Live) Status() types.Status {
	status := live.status.Load()

	if status == nil {
		return types.INITIALIZING
	}

	return status.(types.Status)
}

func (live *Live) Client() *spot.WebSocket {
	return live.client
}

func (live *Live) On(
	channel string, action func([]byte),
) uint64 {
	if live == nil || action == nil {
		return 0
	}

	id := live.nextID.Add(1)
	handler := channelHandler{id: id, fn: action}

	live.handlerMu.Lock()
	defer live.handlerMu.Unlock()

	callbacks, ok := live.sync.Load(channel)

	if !ok {
		live.sync.Store(channel, []channelHandler{handler})
		return id
	}

	live.sync.Store(channel, append(callbacks.([]channelHandler), handler))

	return id
}

/*
Unsubscribe removes one handler previously registered with On for channel.
*/
func (live *Live) Unsubscribe(channel string, id uint64) {
	if live == nil || id == 0 {
		return
	}

	live.handlerMu.Lock()
	defer live.handlerMu.Unlock()

	callbacks, ok := live.sync.Load(channel)

	if !ok {
		return
	}

	live.sync.Store(channel, dropHandler(callbacks.([]channelHandler), id))
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
			errnie.Internal,
			"Kraken REST request failed",
			err,
		))
	}

	errors := utils.GetStringSlice(resp.Body, "error")

	if len(errors) > 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			errors[0],
			nil,
		))
	}

	if resp.StatusCode != 200 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"websocket.Live.do[%d]: %s",
				resp.StatusCode,
				resp.Body,
			),
			nil,
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
	auth := live.transport != nil && live.transport.auth

	return live.do(spot.RequestOptions{
		Auth:   auth,
		Path:   path,
		Method: "GET",
		Query:  params,
	})
}

func (live *Live) Post(
	path string, params json.Marshaler,
) ([]byte, error) {
	auth := live.transport != nil && live.transport.auth

	return live.do(spot.RequestOptions{
		Auth:   auth,
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

	if live.client.IsActive() {
		errnie.Error(live.client.Disconnect())
	}
}
