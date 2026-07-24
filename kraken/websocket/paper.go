package websocket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Paper is the simulated spot websocket and REST transport. It shells out to the
native `kraken paper` CLI so balances, fills, and history stay owned by the
venue ledger under Application Support — not an in-process invented matcher.
Private frames enter Actor roots so Desk and tests Subscribe the same way Live
exposes ticker/book/trade.
*/
type Paper struct {
	*types.Actor
	ctx       context.Context
	cancel    context.CancelFunc
	simulator *Simulator
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

	paper := &Paper{
		ctx:       ctx,
		cancel:    cancel,
		simulator: simulator,
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
Initialize is a no-op; readiness follows the injected simulator.
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
Write routes the same subscription and order envelopes used by live transports
through the paper CLI under simulator latency.
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
	if paper.Actor != nil {
		_ = paper.Actor.Close()
	}

	paper.cancel()
}

/*
TradesHistory loads paper fills from `kraken paper history`.
*/
func (paper *Paper) TradesHistory() (*kraken.TradesHistory, error) {
	var (
		model datura.Map[any]
		err   error
	)

	paper.simulator.Do(REST, func() {
		model, err = paper.execute("history", "history")
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get trades history",
			err,
		))
	}

	return kraken.NewTradesHistoryFromMap(model), nil
}

/*
AddOrder places through `kraken paper buy|sell` under simulator latency.
*/
func (paper *Paper) AddOrder(order *kraken.MarketOrder) error {
	command := []string{
		order.Params.Side,
		order.Params.Symbol,
		order.Params.OrderQty.String(),
	}

	if order.Params.OrderType == "limit" {
		command = append(
			command,
			"--type", "limit",
			"--price", order.Params.LimitPrice.String(),
		)
	}

	var model datura.Map[any]
	var err error

	paper.simulator.Do(REST, func() {
		model, err = paper.execute("executions", command...)
	})

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place paper order",
			err,
		))
	}

	model["pair"] = order.Params.Symbol

	return paper.Place(model, order.ReqID)
}

func (paper *Paper) execute(entity string, command ...string) (datura.Map[any], error) {
	input := []string{"paper"}
	input = append(input, command...)
	input = append(input, "--output", "json")

	cmd := exec.CommandContext(paper.ctx, "kraken", input...)

	if errors.Is(cmd.Err, exec.ErrDot) {
		cmd.Err = nil
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.Output()

	if err != nil {
		details := strings.TrimSpace(stderr.String())

		if details == "" {
			details = strings.TrimSpace(string(stdout))
		}

		if details == "" {
			details = err.Error()
		}

		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"kraken paper "+entity+" command failed: "+details,
			err,
		))
	}

	model := datura.Map[any]{}

	if err := sonic.Unmarshal(stdout, &model); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to decode kraken paper "+entity,
			err,
		))
	}

	if errCategory, ok := model["error"].(string); ok && errCategory != "" {
		message, _ := model["message"].(string)

		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken paper: "+message,
			nil,
		))
	}

	return model, nil
}

/*
Balance emits a paper wallet frame from `kraken paper balance`.
*/
func (paper *Paper) Balance(frameType string) error {
	var model datura.Map[any]
	var err error

	paper.simulator.Do(REST, func() {
		model, err = paper.execute("balances", "balance")
	})

	if err != nil {
		return err
	}

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

	// Paper balance is a full wallet dump that omits zero assets. Emitting it
	// as an incremental update leaves stale positive rows in Balance and keeps
	// phantom OPEN lots.
	return paper.Balance("snapshot")
}
