package paper

import (
	"container/ring"
	"context"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/rawbus"
)

type paperFillDrainStub struct {
	executions []user.Execution
	wallet     user.Balances
}

func (stub *paperFillDrainStub) Send(*qpool.QValue[any]) *types.SocketMessage {
	return nil
}

func (stub *paperFillDrainStub) Observe(...types.Socket) {}

func (stub *paperFillDrainStub) DrainExecutions() []user.Execution {
	rows := append([]user.Execution(nil), stub.executions...)
	stub.executions = stub.executions[:0]

	return rows
}

func (stub *paperFillDrainStub) Wallet() user.Balances {
	return stub.wallet
}

func (stub *paperFillDrainStub) UpdateTicker(*market.TickerUpdate) bool {
	return true
}

func testLatencyRing(latency time.Duration) *ring.Ring {
	latencyRing := ring.New(1)
	latencyRing.Value = latency

	return latencyRing
}

func TestWebSocketConnectIsDeterministicByDefault(test *testing.T) {
	testconfig.Load(test)

	failures, failureErr := newPaperFailureInjectionFromConfig()

	if failureErr != nil {
		test.Fatalf("failure config: %v", failureErr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ws := &WebSocket{
		ctx:       ctx,
		cancel:    cancel,
		latencies: testLatencyRing(time.Nanosecond),
		failures:  failures,
	}

	for attemptIndex := 0; attemptIndex < 64; attemptIndex++ {
		ws.isConnected.Store(false)

		connectErr := ws.Connect(public.EndpointType(baseURL), uint64(attemptIndex))

		if connectErr != nil {
			test.Fatalf("connect attempt %d: %v", attemptIndex, connectErr)
		}

		if !ws.isConnected.Load() {
			test.Fatalf("connect attempt %d did not mark websocket connected", attemptIndex)
		}
	}
}

func TestWebSocketConnectFailureInjection(test *testing.T) {
	testconfig.Load(test)
	viper.Set("trading.paper.failure_injection.enabled", true)
	viper.Set("trading.paper.failure_injection.connect_failure_rate", 1.0)
	viper.Set("trading.paper.failure_injection.disconnect_rate", 0.0)
	viper.Set("trading.paper.failure_injection.seed", 1)

	failures, failureErr := newPaperFailureInjectionFromConfig()

	if failureErr != nil {
		test.Fatalf("failure config: %v", failureErr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ws := &WebSocket{
		ctx:       ctx,
		cancel:    cancel,
		latencies: testLatencyRing(time.Nanosecond),
		failures:  failures,
	}

	connectErr := ws.Connect(public.EndpointType(baseURL), 0)

	if connectErr == nil {
		test.Fatal("expected injected connect failure")
	}

	if ws.isConnected.Load() {
		test.Fatal("injected connect failure marked websocket connected")
	}
}

func TestWebSocketHandleErrorsMalformedService(test *testing.T) {
	message := types.NewSocketMessage()
	message.Errors = []string{"EService"}

	ws := &WebSocket{
		ctx: context.Background(),
	}

	ws.handleErrors(message)
}

func TestWebSocketReadMarketMarksPublishesBalances(test *testing.T) {
	testconfig.Load(test)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := qpool.NewQ[any](ctx, 1, 8, nil)
	subscriber := internal.NewBus(
		ctx,
		pool,
		[]internal.Channel{internal.ChannelUI},
		[]internal.Subscription{
			internal.Subscribe(internal.ChannelUI, "paper-mark-ui-test"),
		},
	)
	ws := &WebSocket{
		bus: internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelRaw, internal.ChannelUI},
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelRaw, "paper-mark-raw-test"),
			},
		),
		sockets: map[string]types.Socket{
			"balances": &paperFillDrainStub{
				wallet: user.Balances{
					Asset: []user.Balance{{
						Asset:   "USD",
						Balance: 201,
					}},
				},
			},
		},
	}

	ws.readMarketMarks()
	rawbus.Send(ws.bus, rawbus.TypeTicker, &market.TickerUpdates{{
		Symbol: "BTC/USD",
		Last:   100,
	}})

	uiFrame, uiErr := subscriber.Receive(internal.ChannelUI)

	if uiErr != nil {
		test.Fatalf("ui receive: %v", uiErr)
	}

	if uiFrame.Type != "balances" {
		test.Fatalf("expected balances UI frame, got %q", uiFrame.Type)
	}
}

func TestWebSocketPublishPaperFills(t *testing.T) {
	testconfig.Load(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := qpool.NewQ[any](ctx, 1, 8, nil)
	subscriber := internal.NewBus(
		ctx,
		pool,
		[]internal.Channel{internal.ChannelRaw, internal.ChannelUI},
		[]internal.Subscription{
			internal.Subscribe(internal.ChannelRaw, "paper-fill-raw-test"),
			internal.Subscribe(internal.ChannelUI, "paper-fill-ui-test"),
		},
	)
	ws := &WebSocket{
		bus: internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelRaw, internal.ChannelUI},
			nil,
		),
		sockets: map[string]types.Socket{
			"orders": &paperFillDrainStub{
				executions: []user.Execution{{
					ClOrdID: "order-1",
					Symbol:  "BTC/USD",
				}},
				wallet: user.Balances{
					Asset: []user.Balance{{
						Asset:   "USD",
						Balance: 160,
					}},
				},
			},
		},
	}

	ws.publishPaperFills()

	rawFrame, rawErr := subscriber.Receive(internal.ChannelRaw)

	if rawErr != nil {
		t.Fatalf("raw receive: %v", rawErr)
	}

	executions, decodeErr := rawbus.DecodeExecutions(rawFrame)

	if decodeErr != nil {
		t.Fatalf("decode executions: %v", decodeErr)
	}

	if len(executions) != 1 {
		t.Fatalf("expected one execution, got %d", len(executions))
	}

	uiFrame, uiErr := subscriber.Receive(internal.ChannelUI)

	if uiErr != nil {
		t.Fatalf("ui receive: %v", uiErr)
	}

	if uiFrame.Type != "balances" {
		t.Fatalf("expected balances UI frame, got %q", uiFrame.Type)
	}
}
