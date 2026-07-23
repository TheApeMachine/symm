package mockapi

import (
	"context"
	"encoding/json"
	"io"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
MockConn is a controllable Kraken transport composed from deterministic
scheduling and Actor roots while implementing websocket.Conn.
*/
type MockConn struct {
	*Control
	*Scheduler
	*types.Actor
	client  *spot.WebSocket
	allowed map[string]struct{}
	paperMu sync.Mutex
	paper   *Paper
	roots   map[string]*types.Subscription[any]
}

/*
Client returns the underlying REST normalizer client.
*/
func (conn *MockConn) Client() *spot.WebSocket {
	return conn.client
}

/*
NewConn creates an isolated transport fake for the supplied symbol universe.
*/
func NewConn(symbols ...string) *MockConn {
	if viper.GetInt("system.actor.buffer") < 1 {
		viper.Set("system.actor.buffer", 64)
	}

	allowed := make(map[string]struct{}, len(symbols))

	for _, symbol := range symbols {
		allowed[symbol] = struct{}{}
	}

	conn := &MockConn{
		Control:   newControl(),
		Scheduler: newScheduler(),
		client:    mockNormalizerClient(symbols),
		allowed:   allowed,
		roots: map[string]*types.Subscription[any]{
			"ticker":     rootSubscription(),
			"book":       rootSubscription(),
			"trade":      rootSubscription(),
			"instrument": rootSubscription(),
			"balances":   rootSubscription(),
			"executions": rootSubscription(),
			"add_order":  rootSubscription(),
			"level3":     rootSubscription(),
		},
	}

	conn.Actor = types.NewActor(context.Background(), nil)

	for name, root := range conn.roots {
		conn.AddRoot(name, root)
	}

	return conn
}

func rootSubscription() *types.Subscription[any] {
	buffer := viper.GetInt("system.actor.buffer")

	if buffer < 1 {
		buffer = 64
	}

	return &types.Subscription[any]{
		Channel: make(chan any, buffer),
	}
}

/*
Emit Sends decoded frames into Actor roots matching Live's typed ingress.
*/
func (conn *MockConn) Emit(channel string, payload []byte) {
	conn.Scheduler.Emit(channel, payload)
	conn.feed(channel, payload)
}

/*
Drain delivers every queued frame through MockConn.Emit so Actor roots see
the same tape as production OnReceived → Send.
*/
func (conn *MockConn) Drain() error {
	if conn == nil || conn.Scheduler == nil {
		return io.ErrClosedPipe
	}

	conn.Scheduler.mu.Lock()

	if conn.Scheduler.closed {
		conn.Scheduler.mu.Unlock()
		return io.ErrClosedPipe
	}

	queued := append([]outbound(nil), conn.Scheduler.queue...)
	conn.Scheduler.queue = nil
	conn.Scheduler.mu.Unlock()

	for _, frame := range queued {
		conn.Emit(frame.channel, frame.payload)
	}

	return nil
}

func (conn *MockConn) feed(channel string, raw []byte) {
	root, ok := conn.roots[channel]

	if !ok {
		return
	}

	switch channel {
	case "ticker":
		frame := kraken.NewTicker(raw)

		if errnie.Error(kraken.Validate(frame)) != nil {
			return
		}

		root.Send(frame)
	case "book":
		frame := kraken.NewBook(raw)

		if errnie.Error(kraken.Validate(frame)) != nil {
			return
		}

		root.Send(frame)
	case "trade":
		frame := kraken.NewTrade(raw)

		if errnie.Error(kraken.Validate(frame)) != nil {
			return
		}

		root.Send(frame)
	case "instrument":
		root.Send(kraken.NewInstrument(raw))
	default:
		root.Send(raw)
	}
}

/*
Write validates and records one websocket request, then queues order results or
delivers the current subscription snapshot through the injected boundary.
*/
func (conn *MockConn) Write(params json.Marshaler) error {
	if conn == nil || params == nil {
		return errnie.Err(errnie.Validation, "tests/mockapi: websocket request required", nil)
	}

	if !conn.Active() {
		return io.ErrClosedPipe
	}

	raw, err := params.MarshalJSON()

	if err != nil {
		return errnie.Err(errnie.Validation, "tests/mockapi: request marshal failed", err)
	}

	request, symbols, err := decodeRequest(raw)

	if err != nil {
		return err
	}

	if err := conn.record(raw); err != nil {
		return err
	}

	if err := conn.validate(request, symbols); err != nil {
		return err
	}

	if request.Method == "add_order" {
		return conn.order(raw)
	}

	if request.Method == "unsubscribe" {
		conn.unsubscribe(request.Params.Channel, symbols)
		return nil
	}

	responses, current := conn.subscribe(request.Params.Channel, symbols)

	if current != nil {
		responses = [][]byte{current()}
	}

	for _, response := range responses {
		filtered, matched, err := filterSymbols(
			request.Params.Channel,
			response,
			symbols,
		)

		if err != nil {
			return err
		}

		if matched {
			conn.Emit(request.Params.Channel, filtered)
		}
	}

	return nil
}

/*
Publish queues one market update filtered through the connection's current
symbol subscription rather than bypassing the venue boundary.
*/
func (conn *MockConn) Publish(channel string, payload []byte) error {
	if !conn.Active() {
		return io.ErrClosedPipe
	}

	if !conn.Subscribed(channel) {
		return nil
	}

	symbols := conn.Subscriptions(channel)

	filtered, matched, err := filterSymbols(channel, payload, symbols)

	if err != nil {
		return err
	}

	if !matched {
		return nil
	}

	return conn.Queue(channel, filtered)
}

/*
Close disables all future scheduling and releases configured fixture state.
*/
func (conn *MockConn) Close() {
	if conn == nil {
		return
	}

	conn.Scheduler.close()
	conn.Control.mu.Lock()
	conn.Control.responses = nil
	conn.Control.current = nil
	conn.Control.subscriptions = nil
	conn.Control.mu.Unlock()
	conn.paperMu.Lock()
	conn.paper = nil
	conn.paperMu.Unlock()
}

/*
Post records one REST request and returns only an explicitly configured route.
*/
func (conn *MockConn) Post(path string, params json.Marshaler) ([]byte, error) {
	if conn == nil || params == nil {
		return nil, errnie.Err(errnie.Validation, "tests/mockapi: REST request required", nil)
	}

	if !conn.Active() {
		return nil, io.ErrClosedPipe
	}

	raw, err := params.MarshalJSON()

	if err != nil {
		return nil, errnie.Err(errnie.Validation, "tests/mockapi: REST marshal failed", err)
	}

	return conn.post(path, raw)
}

/*
wireRequest decodes the request fields needed by connection-level validation.
*/
type wireRequest struct {
	Method string `json:"method"`
	Params struct {
		Channel string          `json:"channel"`
		Symbol  json.RawMessage `json:"symbol"`
		Depth   int             `json:"depth"`
	} `json:"params"`
}

/*
validate enforces the subscription contract represented by the fake venue.
*/
func (conn *MockConn) validate(request wireRequest, symbols []string) error {
	if request.Method != "subscribe" && request.Method != "unsubscribe" &&
		request.Method != "add_order" {
		return errnie.Err(errnie.NotImplemented, "tests/mockapi: unknown method "+request.Method, nil)
	}

	if (request.Method == "subscribe" || request.Method == "unsubscribe") &&
		request.Params.Channel == "" {
		return errnie.Err(errnie.Validation, "tests/mockapi: subscription channel required", nil)
	}

	if request.Method == "subscribe" || request.Method == "unsubscribe" {
		channels := map[string]struct{}{
			"instrument": {},
			"ticker":     {},
			"trade":      {},
			"book":       {},
			"level3":     {},
			"balances":   {},
			"executions": {},
		}

		if _, exists := channels[request.Params.Channel]; !exists {
			return errnie.Err(
				errnie.NotFound,
				"tests/mockapi: unknown subscription channel "+request.Params.Channel,
				nil,
			)
		}
	}

	for _, symbol := range symbols {
		_, exists := conn.allowed[symbol]

		if len(conn.allowed) > 0 && !exists {
			return errnie.Err(
				errnie.Validation,
				"tests/mockapi: unknown subscription symbol "+symbol,
				nil,
			)
		}
	}

	if request.Method == "subscribe" && request.Params.Channel == "level3" &&
		request.Params.Depth <= 0 {
		return errnie.Err(errnie.Validation, "tests/mockapi: level3 depth required", nil)
	}

	return nil
}

/*
order delegates accepted add_order requests to the composed paper ledger.
*/
func (conn *MockConn) order(raw []byte) error {
	conn.paperMu.Lock()
	paper := conn.paper
	conn.paperMu.Unlock()

	if paper == nil {
		return errnie.Err(errnie.NotImplemented, "tests/mockapi: paper engine not configured", nil)
	}

	frames, err := paper.Handle(raw)

	if err != nil {
		return err
	}

	for _, frame := range frames {
		if frame.channel != "add_order" && !conn.Subscribed(frame.channel) {
			continue
		}

		if err := conn.Queue(frame.channel, frame.payload); err != nil {
			return err
		}
	}

	return nil
}

/*
decodeRequest preserves symbol shape until the request method is known.
*/
func decodeRequest(raw []byte) (wireRequest, []string, error) {
	request := wireRequest{}

	if err := json.Unmarshal(raw, &request); err != nil {
		return request, nil, errnie.Err(errnie.Validation, "tests/mockapi: request decode failed", err)
	}

	symbols := []string{}

	if (request.Method == "subscribe" || request.Method == "unsubscribe") &&
		len(request.Params.Symbol) > 0 {
		if err := json.Unmarshal(request.Params.Symbol, &symbols); err != nil {
			return request, nil, errnie.Err(
				errnie.Validation,
				"tests/mockapi: subscription symbols must be an array",
				err,
			)
		}
	}

	return request, symbols, nil
}
