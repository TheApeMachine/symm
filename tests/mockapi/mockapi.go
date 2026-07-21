package mockapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
)

/*
MockConn is a controllable Kraken websocket transport that implements
websocket.Conn so tests can inject it through websocket.NewAPI exactly as root
does.
*/
type MockConn struct {
	mu            sync.Mutex
	channels      map[string][]mockHandler
	nextID        uint64
	client        *spot.WebSocket
	writes        [][]byte
	posts         [][]byte
	writeErr      error
	responses     map[string][][]byte
	postResponses map[string][]byte
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
NewConn creates an isolated transport fake with the Kraken SDK normalizer data
required by the real API constructor.
*/
func NewConn() *MockConn {
	return &MockConn{client: mockNormalizerClient()}
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

	request := struct {
		Method string `json:"method"`
		Params struct {
			Channel string `json:"channel"`
		} `json:"params"`
	}{}

	if err := sonic.Unmarshal(raw, &request); err != nil {
		return errnie.Err(
			errnie.Validation,
			"tests/mockapi: websocket request decode failed",
			err,
		)
	}

	conn.mu.Lock()
	conn.writes = append(conn.writes, append([]byte(nil), raw...))
	err = conn.writeErr
	responses := append([][]byte(nil), conn.responses[request.Params.Channel]...)
	conn.mu.Unlock()

	if request.Method == "subscribe" {
		for _, response := range responses {
			conn.Emit(request.Params.Channel, response)
		}
	}

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
	raw, err := params.MarshalJSON()

	if err != nil {
		return nil, errnie.Err(
			errnie.Validation,
			"tests/mockapi: REST request marshal failed",
			err,
		)
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()
	conn.posts = append(conn.posts, append([]byte(nil), raw...))

	return append([]byte(nil), conn.postResponses[path]...), nil
}

/*
RespondPost associates one REST endpoint with the ready Kraken payload returned
by this fake connection.
*/
func (conn *MockConn) RespondPost(path string, payload []byte) {
	if conn == nil || path == "" || len(payload) == 0 {
		panic(errnie.Err(
			errnie.Validation,
			"tests/mockapi: REST path and payload are required",
			nil,
		))
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()

	if conn.postResponses == nil {
		conn.postResponses = map[string][]byte{}
	}

	conn.postResponses[path] = append([]byte(nil), payload...)
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
Respond associates a Kraken subscription channel with the snapshot that the
mock transport emits when production subscribes to that channel.
*/
func (conn *MockConn) Respond(channel string, payload []byte) {
	if conn == nil || channel == "" || len(payload) == 0 {
		panic(errnie.Err(
			errnie.Validation,
			"tests/mockapi: response channel and payload are required",
			nil,
		))
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()

	if conn.responses == nil {
		conn.responses = map[string][][]byte{}
	}

	conn.responses[channel] = append(
		conn.responses[channel],
		append([]byte(nil), payload...),
	)
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
Posts returns independent copies of the REST request bodies in call order.
*/
func (conn *MockConn) Posts() [][]byte {
	if conn == nil {
		return nil
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()

	posts := make([][]byte, len(conn.posts))

	for index := range conn.posts {
		posts[index] = append([]byte(nil), conn.posts[index]...)
	}

	return posts
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
