package websocket

import (
	"context"
	"encoding/json"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Paper is the simulated spot websocket and REST transport.
Private frames enter Actor roots so Desk and tests Subscribe the same way
Live exposes ticker/book/trade. Orders match in-process through Matcher.
*/
type Paper struct {
	*types.Actor
	ctx       context.Context
	cancel    context.CancelFunc
	simulator *Simulator
	matcher   *Matcher
	roots     map[string]*types.Subscription[any]
}

var _ Conn = (*Paper)(nil)

/*
NewPaper opens the paper spot transport with Actor roots for private channels.
*/
func NewPaper(
	ctx context.Context,
	simulator *Simulator,
	cfg config.Config,
) *Paper {
	ctx, cancel := context.WithCancel(ctx)

	quote := cfg.Market.QuoteCurrency

	if quote == "" {
		quote = "USD"
	}

	clock := Clock(WallClock{})

	if simulator != nil && simulator.clock != nil {
		clock = simulator.clock
	}

	paper := &Paper{
		ctx:       ctx,
		cancel:    cancel,
		simulator: simulator,
		matcher:   NewMatcher(clock, quote, 100_000, 0.0026),
		roots: map[string]*types.Subscription[any]{
			"balances":   paperRoot(cfg.System.ActorBuffer),
			"executions": paperRoot(cfg.System.ActorBuffer),
			"add_order":  paperRoot(cfg.System.ActorBuffer),
		},
	}

	paper.Actor = types.NewActor(ctx, nil)

	for name, root := range paper.roots {
		paper.AddRoot(name, root)
	}

	paper.Actor.Initialize()

	return paper
}

func paperRoot(buffer int) *types.Subscription[any] {
	if buffer < 1 {
		buffer = 64
	}

	return &types.Subscription[any]{
		Channel: make(chan any, buffer),
	}
}

/*
Initialize marks the paper transport ready once its simulator is ready.
*/
func (paper *Paper) Initialize() error {
	return nil
}

/*
Status reports the backing simulator status.
*/
func (paper *Paper) Status() types.Status {
	return paper.simulator.Status()
}

/*
Client satisfies Conn; paper transport never owns a venue REST client.
*/
func (paper *Paper) Client() *spot.WebSocket {
	return nil
}

/*
Matcher exposes the in-process exchange for tests that seed marks or balances.
*/
func (paper *Paper) Matcher() *Matcher {
	return paper.matcher
}

/*
Write routes the same subscription and order envelopes used by live transports
through the paper simulator.
*/
func (paper *Paper) Write(params json.Marshaler) error {
	raw, err := params.MarshalJSON()

	if err != nil {
		return err
	}

	request := struct {
		Method string `json:"method"`
		Params struct {
			Channel string `json:"channel"`
		} `json:"params"`
	}{}

	if err := sonic.Unmarshal(raw, &request); err != nil {
		return err
	}

	if request.Method == "add_order" {
		order := &kraken.MarketOrder{}

		if err := sonic.Unmarshal(raw, order); err != nil {
			return err
		}

		return paper.AddOrder(order)
	}

	switch request.Params.Channel {
	case "balances":
		return paper.Balance("snapshot")
	case "executions":
		return nil
	default:
		return types.ClosedError{Component: "paper:" + request.Params.Channel}
	}
}

/*
Post satisfies Conn; paper REST operations remain explicit typed methods.
*/
func (paper *Paper) Post(string, json.Marshaler) ([]byte, error) {
	return nil, types.ClosedError{Component: "paper:rest"}
}

/*
Emit Sends one private frame into the matching Actor root.
*/
func (paper *Paper) Emit(channel string, payload json.Marshaler) error {
	raw, err := payload.MarshalJSON()

	if err != nil {
		return err
	}

	root, ok := paper.roots[channel]

	if !ok {
		return types.ClosedError{Component: "paper:" + channel}
	}

	root.Send(raw)

	return nil
}

/*
Root returns the Actor fan-out for private channel publish.
*/
func (paper *Paper) Root() *types.Actor {
	return paper.Actor
}

/*
Close cancels the paper actor context.
*/
func (paper *Paper) Close() {
	paper.cancel()
}

/*
TradesHistory returns an empty history; paper fills are streamed as executions.
*/
func (paper *Paper) TradesHistory() (*kraken.TradesHistory, error) {
	return kraken.NewTradesHistoryFromMap(datura.Map[any]{}), nil
}

/*
AddOrder fills through the in-process matcher under simulator latency.
*/
func (paper *Paper) AddOrder(order *kraken.MarketOrder) error {
	quantity, err := ParseQuantity(string(order.Params.OrderQty))

	if err != nil {
		return err
	}

	limit := 0.0

	if order.Params.OrderType == "limit" && string(order.Params.LimitPrice) != "" {
		limit, err = ParseQuantity(string(order.Params.LimitPrice))

		if err != nil {
			return err
		}
	}

	var model datura.Map[any]

	paper.simulator.Do(REST, func() {
		model, err = paper.matcher.Fill(
			order.Params.Side,
			order.Params.Symbol,
			quantity,
			limit,
		)
	})

	if err != nil {
		return err
	}

	model["pair"] = order.Params.Symbol

	return paper.Place(model, order.ReqID)
}

/*
Balance emits a paper wallet frame through the latency simulator.
*/
func (paper *Paper) Balance(frameType string) error {
	var model datura.Map[any]

	paper.simulator.Do(REST, func() {
		model = paper.matcher.Balances()
	})

	balance := kraken.NewBalanceFromMap(model)
	balance.Type = frameType

	return paper.simulator.Emit(
		paper, WEBSOCKET, "balances", balance,
	)
}

/*
Replay emits historical paper fills as execution frames.
*/
func (paper *Paper) Replay(trades []any) error {
	for tradeIndex, tradeRaw := range trades {
		trade, ok := tradeRaw.(map[string]any)

		if !ok {
			continue
		}

		execution := kraken.NewExecutionFromMap(datura.Map[any](trade))

		if tradeIndex == 0 {
			execution.Type = "snapshot"
		}

		err := paper.simulator.Emit(paper, WEBSOCKET, "executions", execution)

		if err != nil {
			return err
		}
	}

	return nil
}

/*
Place emits order ack, fill, and a balance snapshot for one paper order.
*/
func (paper *Paper) Place(model datura.Map[any], reqID int64) error {
	orderAck := kraken.NewOrderResponseFromMap(model, reqID)

	err := paper.simulator.Emit(paper, WEBSOCKET, "add_order", orderAck)

	if err != nil {
		return err
	}

	err = paper.simulator.Emit(
		paper, WEBSOCKET, "executions", kraken.NewExecutionFromMap(model),
	)

	if err != nil {
		return err
	}

	return paper.Balance("snapshot")
}
