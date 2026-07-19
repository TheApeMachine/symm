package mockapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	krakenws "github.com/theapemachine/symm/kraken/websocket"
)

// Compile-time proof that MockConn is a drop-in websocket.Conn emulator.
var _ krakenws.Conn = (*MockConn)(nil)

/*
MockConn is a controllable Kraken websocket transport that implements
websocket.Conn so tests can inject it through websocket.NewAPI exactly as root
does.
*/
type MockConn struct {
	mu           sync.Mutex
	channels     map[string][]mockHandler
	nextID       uint64
	client       *spot.WebSocket
	writes       [][]byte
	writeErr     error
	postResponse []byte
	postSymbols  []string
}

type mockHandler struct {
	id uint64
	fn func([]byte)
}

/*
Client returns the underlying REST normalizer client.
*/
func (conn *MockConn) Client() *spot.WebSocket {
	return conn.client
}

/*
On registers a channel handler that receives emitted payloads and returns the
subscription id used by Unsubscribe.
*/
func (conn *MockConn) On(channel string, action func([]byte)) uint64 {
	if conn == nil || action == nil {
		return 0
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()

	if conn.channels == nil {
		conn.channels = map[string][]mockHandler{}
	}

	conn.nextID++
	id := conn.nextID
	conn.channels[channel] = append(conn.channels[channel], mockHandler{
		id: id,
		fn: action,
	})

	return id
}

/*
Unsubscribe removes one handler previously registered with On by subscription
id so position teardown can drop ticker listeners without clearing unrelated
consumers.
*/
func (conn *MockConn) Unsubscribe(channel string, id uint64) {
	if conn == nil || id == 0 {
		return
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()

	if conn.channels == nil {
		return
	}

	handlers := conn.channels[channel]

	if len(handlers) == 0 {
		return
	}

	next := make([]mockHandler, 0, len(handlers))

	for _, handler := range handlers {
		if handler.id == id {
			continue
		}

		next = append(next, handler)
	}

	conn.channels[channel] = next
}

/*
Write records one outbound websocket request and returns the configured error.
Tests can therefore verify that production subscriptions and orders crossed the
injected Conn boundary instead of disappearing into an SDK client.
*/
func (conn *MockConn) Write(params json.Marshaler) error {
	if conn == nil || params == nil {
		return errnie.Err(
			errnie.Validation,
			"tests/mockapi: websocket request is required",
			nil,
		)
	}

	raw, err := params.MarshalJSON()

	if err != nil {
		return errnie.Err(
			errnie.Validation,
			"tests/mockapi: websocket request marshal failed",
			err,
		)
	}

	conn.mu.Lock()
	conn.writes = append(conn.writes, append([]byte(nil), raw...))
	err = conn.writeErr
	conn.mu.Unlock()

	return err
}

/*
Close satisfies Conn; the in-memory producer owns no external resources.
*/
func (conn *MockConn) Close() {}

/*
Post records the REST call and returns the configured response.
*/
func (conn *MockConn) Post(path string, params json.Marshaler) ([]byte, error) {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	if path == krakenws.TradeVolumeEndpoint {
		raw, err := params.MarshalJSON()

		if err != nil {
			return nil, errnie.Err(
				errnie.Validation,
				"tests/mockapi: trade volume request marshal failed",
				err,
			)
		}

		request := &kraken.TradeVolumeRequest{}

		if err := sonic.Unmarshal(raw, request); err != nil {
			return nil, errnie.Err(
				errnie.Validation,
				"tests/mockapi: trade volume request decode failed",
				err,
			)
		}

		conn.postSymbols = strings.Split(request.Pair, ",")
	}

	return conn.postResponse, nil
}

/*
Emit delivers one payload to every handler registered for channel.
*/
func (conn *MockConn) Emit(channel string, payload []byte) {
	conn.mu.Lock()
	handlers, ok := conn.channels[channel]

	if !ok {
		conn.mu.Unlock()
		return
	}

	handlers = append([]mockHandler(nil), handlers...)
	conn.mu.Unlock()

	for _, handler := range handlers {
		handler.fn(payload)
	}
}

/*
Writes returns independent copies of the websocket requests sent through this
connection, preserving their production order for assertions.
*/
func (conn *MockConn) Writes() [][]byte {
	if conn == nil {
		return nil
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()

	writes := make([][]byte, len(conn.writes))

	for index := range conn.writes {
		writes[index] = append([]byte(nil), conn.writes[index]...)
	}

	return writes
}

/*
FailWrites configures the websocket error returned after each request is
recorded, allowing package tests to prove subscription failures are propagated.
*/
func (conn *MockConn) FailWrites(err error) {
	if conn == nil {
		return
	}

	conn.mu.Lock()
	conn.writeErr = err
	conn.mu.Unlock()
}

/*
MockAPI wires a controllable websocket.API for integration tests.
*/
type MockAPI struct {
	public  *MockConn
	private *MockConn
}

/*
NewMockAPI constructs controllable public and private Conn emulators.
*/
func NewMockAPI() *MockAPI {
	return &MockAPI{
		public:  &MockConn{client: mockNormalizerClient()},
		private: &MockConn{client: mockNormalizerClient()},
	}
}

/*
Public returns the public transport mock.
*/
func (mock *MockAPI) Public() *MockConn {
	return mock.public
}

/*
Private returns the private transport mock.
*/
func (mock *MockAPI) Private() *MockConn {
	return mock.private
}

/*
Wire returns a paper-mode websocket.API backed by this emulator, matching the
Conn injection path used in cmd/root.go. trading.model is forced to paper for
the lifetime of the returned API construction.
*/
func (mock *MockAPI) Wire(
	ctx context.Context,
) (*krakenws.API, *krakenws.Paper, error) {
	if mock == nil {
		return nil, nil, errnie.Err(
			errnie.Validation, "tests/mockapi: mock API is required", nil,
		)
	}

	simulator := krakenws.NewSimulator()

	if err := simulator.Initialize(); err != nil {
		return nil, nil, errnie.Err(
			errnie.Internal,
			"tests/mockapi: simulator initialize failed",
			err,
		)
	}

	paper := krakenws.NewPaper(ctx, simulator)

	if err := paper.Initialize(); err != nil {
		return nil, nil, errnie.Err(
			errnie.Internal,
			"tests/mockapi: paper initialize failed",
			err,
		)
	}

	previous := viper.GetString("trading.model")
	viper.Set("trading.model", "paper")
	api := krakenws.NewAPI(ctx, mock.public, mock.private, paper)
	viper.Set("trading.model", previous)

	return api, paper, nil
}

/*
Emit delivers one public channel frame to every registered handler, the same
path Market and Price use after API.On registration.
*/
func (mock *MockAPI) Emit(channel string, payload []byte) {
	if mock == nil {
		return
	}

	mock.public.Emit(channel, payload)
}

/*
SetTradeVolumeResponse configures the private TradeVolume REST response.
*/
func (mock *MockAPI) SetTradeVolumeResponse(tradeVolume *kraken.TradeVolume) error {
	raw, err := sonic.Marshal(tradeVolume)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"tests/mockapi: trade volume marshal failed",
			err,
		))
	}

	mock.private.mu.Lock()
	mock.private.postResponse = raw
	mock.private.mu.Unlock()

	return nil
}

/*
LastTradeVolumeSymbols returns the pair list from the latest TradeVolume post.
*/
func (mock *MockAPI) LastTradeVolumeSymbols() ([]string, error) {
	mock.private.mu.Lock()
	symbols := append([]string(nil), mock.private.postSymbols...)
	mock.private.mu.Unlock()

	if len(symbols) == 1 && symbols[0] == "" {
		return nil, nil
	}

	return symbols, nil
}

func mockNormalizerClient() *spot.WebSocket {
	client := spot.NewWebSocket()
	client.REST.Executor = func(request *http.Request) (*http.Response, error) {
		version := request.URL.Query().Get("assetVersion")
		body := `{"error":[],"result":{}}`

		switch request.URL.Path {
		case "/0/public/Assets":
			body = `{"error":[],"result":{"XXBT":{"altname":"XBT"},"XZEC":{"altname":"ZEC"},"ZUSD":{"altname":"USD"}}}`

			if version == "1" {
				body = `{"error":[],"result":{"BTC":{"altname":"XBT"},"ZEC":{"altname":"ZEC"},"USD":{"altname":"USD"}}}`
			}
		case "/0/public/AssetPairs":
			body = `{"error":[],"result":{"XXBTZUSD":{"wsname":"XBT/USD","base":"XXBT","quote":"ZUSD"},"XZECZUSD":{"wsname":"ZEC/USD","base":"XZEC","quote":"ZUSD"}}}`

			if version == "1" {
				body = `{"error":[],"result":{"BTC/USD":{"wsname":"BTC/USD","base":"BTC","quote":"USD"},"ZEC/USD":{"wsname":"ZEC/USD","base":"ZEC","quote":"USD"}}}`
			}
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	}

	return client
}
