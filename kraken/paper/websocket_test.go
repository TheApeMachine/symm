package paper

import (
	"context"
	"testing"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/internal/testconfig"
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
