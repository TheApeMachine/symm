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
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	models "github.com/theapemachine/symm/kraken"
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
	status        atomic.Value
	ctx           context.Context
	cancel        context.CancelFunc
	client        *spot.WebSocket
	sync          *sync.Map
	handlerMu     sync.Mutex
	nextID        atomic.Uint64
	paper         *Paper
	simulator     *Simulator
	books         *spot.BookManager
	bookMu        sync.RWMutex
	isLevel3      bool
	symbols       []string
	transport     *Transport
	level3Queue   chan []byte
	level3Ledger  *level3Ledger
	tickerChannel     chan []models.TickerData
	bookChannel       chan []models.BookData
	tradeChannel      chan []models.TradeData
	balancesChannel   chan *models.Balance
	executionsChannel chan *models.Execution
	instrumentChannel chan models.InstrumentData
	orderChannel      chan *models.OrderResponse
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
		ctx:           ctx,
		cancel:        cancel,
		simulator:     simulator,
		client:        spot.NewWebSocket(),
		sync:          &sync.Map{},
		transport:     transport,
		tickerChannel:     make(chan []models.TickerData, 256),
		bookChannel:       make(chan []models.BookData, 256),
		tradeChannel:      make(chan []models.TradeData, 256),
		balancesChannel:   make(chan *models.Balance, 256),
		executionsChannel: make(chan *models.Execution, 256),
		instrumentChannel: make(chan models.InstrumentData, 256),
		orderChannel:      make(chan *models.OrderResponse, 256),
	}
	live.status.Store(types.INITIALIZING)
	live.client.URL = endpoint

	if endpoint == Level3WebSocketURL {
		live.isLevel3 = true
		configureLevel3(live)
	}

	transport.wireCredentials(live.client)

	live.client.OnReceived.Recurring(func(e *callback.Event[*kraken.WebSocketMessage]) {
		mapped, err := e.Data.Map()

		if err != nil {
			errnie.Error(errnie.Err(errnie.Internal, "failed to map websocket message", err))
			return
		}

		switch mapped["channel"] {
		case "ticker":
			live.tickerChannel <- mapped["data"].([]models.TickerData)
		case "book":
			live.bookChannel <- mapped["data"].([]models.BookData)
		case "trade":
			live.tradeChannel <- mapped["data"].([]models.TradeData)
		case "balances":
			live.balancesChannel <- models.NewBalance(e.Data.Bytes())
		case "executions":
			live.executionsChannel <- models.NewExecution(e.Data.Bytes())
		case "instrument":
			live.instrumentChannel <- models.NewInstrumentData(e.Data.Bytes())
		default:
			if mapped["method"] == "add_order" {
				live.orderChannel <- models.NewOrderResponse(e.Data.Bytes())
				return
			}

			errnie.Error(errnie.Err(errnie.Internal, "unknown websocket channel", nil))
		}
	})
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

func (live *Live) TickerChannel() chan []models.TickerData {
	return live.tickerChannel
}

func (live *Live) BookChannel() chan []models.BookData {
	return live.bookChannel
}

func (live *Live) TradeChannel() chan []models.TradeData {
	return live.tradeChannel
}

func (live *Live) BalancesChannel() chan *models.Balance {
	return live.balancesChannel
}

func (live *Live) ExecutionsChannel() chan *models.Execution {
	return live.executionsChannel
}

func (live *Live) InstrumentChannel() chan models.InstrumentData {
	return live.instrumentChannel
}

func (live *Live) OrderChannel() chan *models.OrderResponse {
	return live.orderChannel
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
