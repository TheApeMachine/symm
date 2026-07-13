package tests

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	krakenws "github.com/theapemachine/symm/kraken/websocket"
)

/*
MockConn is a controllable Kraken websocket transport.
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
	level3  *MockConn
}

/*
NewMockAPI constructs a paper-mode API backed by controllable transports.
*/
func NewMockAPI() *MockAPI {
	public := &MockConn{client: mockNormalizerClient()}
	private := &MockConn{}
	level3 := &MockConn{}

	return &MockAPI{
		public:  public,
		private: private,
		level3:  level3,
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
Level3 returns the level3 transport mock.
*/
func (mock *MockAPI) Level3() *MockConn {
	return mock.level3
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

	symbols := make([]string, 0, len(request.Pair))

	for _, pair := range request.Pair {
		symbols = append(symbols, pair.Asset)
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
			body = `{"error":[],"result":{"XXBT":{"altname":"XBT"},"ZUSD":{"altname":"USD"}}}`

			if version == "1" {
				body = `{"error":[],"result":{"BTC":{"altname":"XBT"},"USD":{"altname":"USD"}}}`
			}
		case "/0/public/AssetPairs":
			body = `{"error":[],"result":{"XXBTZUSD":{"wsname":"XBT/USD","base":"XXBT","quote":"ZUSD"}}}`

			if version == "1" {
				body = `{"error":[],"result":{"BTC/USD":{"wsname":"BTC/USD","base":"BTC","quote":"USD"}}}`
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
