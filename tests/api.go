package tests

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

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
websocket.Conn so tests inject it through NewAPI exactly as root does.
*/
type MockConn struct {
	channels     map[string][]func([]byte)
	client       *spot.WebSocket
	postResponse []byte
	postPath     string
	postParams   json.Marshaler
	postCalls    []MockPostCall
	closeCount   int
}

/*
MockPostCall records one REST post issued through a mock transport.
*/
type MockPostCall struct {
	Path   string
	Params json.Marshaler
}

func (conn *MockConn) Client() *spot.WebSocket {
	return conn.client
}

func (conn *MockConn) On(channel string, action func([]byte)) {
	if conn.channels == nil {
		conn.channels = map[string][]func([]byte){}
	}

	conn.channels[channel] = append(conn.channels[channel], action)
}

func (conn *MockConn) Write(params json.Marshaler) error {
	return nil
}

func (conn *MockConn) Close() {
	conn.closeCount++
}

func (conn *MockConn) Post(path string, params json.Marshaler) ([]byte, error) {
	conn.postPath = path
	conn.postParams = params
	conn.postCalls = append(conn.postCalls, MockPostCall{
		Path:   path,
		Params: params,
	})

	return conn.postResponse, nil
}

/*
Emit delivers one payload to every handler registered for channel.
*/
func (conn *MockConn) Emit(channel string, payload []byte) {
	handlers, ok := conn.channels[channel]

	if !ok {
		return
	}

	for _, handler := range handlers {
		handler(payload)
	}
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
			errnie.Validation, "tests: mock API is required", nil,
		)
	}

	simulator := krakenws.NewSimulator()

	if err := simulator.Initialize(); err != nil {
		return nil, nil, errnie.Err(
			errnie.Internal, "tests: simulator initialize failed", err,
		)
	}

	paper := krakenws.NewPaper(ctx, simulator)

	if err := paper.Initialize(); err != nil {
		return nil, nil, errnie.Err(
			errnie.Internal, "tests: paper initialize failed", err,
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
func (mock *MockAPI) Emit(frame Frame) {
	if mock == nil {
		return
	}

	mock.public.Emit(frame.Channel, frame.Payload)
}

/*
SetTradeVolumeResponse configures the private TradeVolume REST response.
*/
func (mock *MockAPI) SetTradeVolumeResponse(tradeVolume *kraken.TradeVolume) error {
	raw, err := sonic.Marshal(tradeVolume)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"tests: trade volume marshal failed",
			err,
		))
	}

	mock.private.postResponse = raw

	return nil
}

/*
LastTradeVolumeSymbols returns the pair list from the latest TradeVolume post.
*/
func (mock *MockAPI) LastTradeVolumeSymbols() []string {
	for index := len(mock.private.postCalls) - 1; index >= 0; index-- {
		call := mock.private.postCalls[index]

		if call.Path != krakenws.TradeVolumeEndpoint {
			continue
		}

		symbols, err := tradeVolumeSymbols(call.Params)

		if err != nil {
			return nil
		}

		return symbols
	}

	return nil
}

func tradeVolumeSymbols(params json.Marshaler) ([]string, error) {
	raw, err := params.MarshalJSON()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"tests: trade volume request marshal failed",
			err,
		))
	}

	request := &kraken.TradeVolumeRequest{}
	err = sonic.Unmarshal(raw, request)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"tests: trade volume request decode failed",
			err,
		))
	}

	if request.Pair == "" {
		return nil, nil
	}

	return strings.Split(request.Pair, ","), nil
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
