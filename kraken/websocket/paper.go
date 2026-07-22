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
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Paper is the simulated spot websocket and REST transport.
*/
type Paper struct {
	ctx           context.Context
	cancel        context.CancelFunc
	status        types.Status
	simulator     *Simulator
	lifecycle     *Lifecycle
	url           string
	auth          bool
	tickerChannel     chan []kraken.TickerData
	bookChannel       chan []kraken.BookData
	tradeChannel      chan []kraken.TradeData
	balancesChannel   chan *kraken.Balance
	executionsChannel chan *kraken.Execution
	instrumentChannel chan kraken.InstrumentData
	orderChannel      chan *kraken.OrderResponse
}

var _ Conn = (*Paper)(nil)

/*
NewPaper opens the paper spot transport.
*/
func NewPaper(
	ctx context.Context,
	simulator *Simulator,
) *Paper {
	ctx, cancel := context.WithCancel(ctx)

	paper := &Paper{
		ctx:           ctx,
		cancel:        cancel,
		status:        types.READY,
		simulator:     simulator,
		tickerChannel:     make(chan []kraken.TickerData, 256),
		bookChannel:       make(chan []kraken.BookData, 256),
		tradeChannel:      make(chan []kraken.TradeData, 256),
		balancesChannel:   make(chan *kraken.Balance, 256),
		executionsChannel: make(chan *kraken.Execution, 256),
		instrumentChannel: make(chan kraken.InstrumentData, 256),
		orderChannel:      make(chan *kraken.OrderResponse, 256),
	}

	paper.lifecycle = NewLifecycle(paper)
	return paper
}

func (paper *Paper) Initialize() error {
	paper.status = types.READY
	return nil
}

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
through the paper simulator.
*/
func (paper *Paper) Write(params json.Marshaler) error {
	raw, err := params.MarshalJSON()

	if err != nil {
		return errnie.Err(errnie.Validation, "paper request marshal failed", err)
	}

	request := struct {
		Method string `json:"method"`
		Params struct {
			Channel string `json:"channel"`
		} `json:"params"`
	}{}

	if err := sonic.Unmarshal(raw, &request); err != nil {
		return errnie.Err(errnie.Validation, "paper request decode failed", err)
	}

	if request.Method == "add_order" {
		order := &kraken.MarketOrder{}

		if err := sonic.Unmarshal(raw, order); err != nil {
			return errnie.Err(errnie.Validation, "paper order decode failed", err)
		}

		return paper.AddOrder(order)
	}

	switch request.Params.Channel {
	case "balances":
		return paper.SubBalances()
	case "executions":
		return paper.SubExecutions()
	default:
		return errnie.Err(errnie.NotImplemented, "paper request is not implemented", nil)
	}
}

/*
Post satisfies Conn; paper REST operations remain explicit typed methods.
*/
func (paper *Paper) Post(string, json.Marshaler) ([]byte, error) {
	return nil, errnie.Err(errnie.NotImplemented, "paper REST post is not implemented", nil)
}

/*
TickerChannel exposes the typed ticker stream produced from emitted frames,
matching the Live transport so downstream signals consume identical data.
*/
func (paper *Paper) TickerChannel() chan []kraken.TickerData {
	return paper.tickerChannel
}

/*
BookChannel exposes the typed book stream produced from emitted frames.
*/
func (paper *Paper) BookChannel() chan []kraken.BookData {
	return paper.bookChannel
}

/*
TradeChannel exposes the typed trade stream produced from emitted frames.
*/
func (paper *Paper) TradeChannel() chan []kraken.TradeData {
	return paper.tradeChannel
}

/*
BalancesChannel exposes the typed private balances stream produced by the paper
lifecycle so the broker consumes venue and paper balances identically.
*/
func (paper *Paper) BalancesChannel() chan *kraken.Balance {
	return paper.balancesChannel
}

/*
ExecutionsChannel exposes the typed private executions stream produced by the
paper lifecycle.
*/
func (paper *Paper) ExecutionsChannel() chan *kraken.Execution {
	return paper.executionsChannel
}

/*
InstrumentChannel exposes the typed instrument metadata stream. The paper
transport produces no instrument frames; the channel exists to satisfy Conn.
*/
func (paper *Paper) InstrumentChannel() chan kraken.InstrumentData {
	return paper.instrumentChannel
}

/*
OrderChannel exposes the typed order-acknowledgement stream produced by the
paper lifecycle for add_order responses.
*/
func (paper *Paper) OrderChannel() chan *kraken.OrderResponse {
	return paper.orderChannel
}

/*
Unsubscribe satisfies Conn. The paper transport delivers frames directly onto
typed channels rather than per-subscription handlers, so there is nothing to
drop; the method is a no-op.
*/
func (paper *Paper) Unsubscribe(channel string, id uint64) {}

/*
Emit delivers one simulated frame. Market-data channels are decoded into the
same typed slices Live produces and pushed onto the matching channel so signals
see identical data. Private lifecycle channels (balances, executions,
add_order, instrument) are decoded with the same constructors Live uses and
pushed onto their typed channels so the broker consumes venue and paper frames
identically.
*/
func (paper *Paper) Emit(channel string, payload json.Marshaler) error {
	raw, err := payload.MarshalJSON()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to emit payload",
			err,
		))
	}

	switch channel {
	case "ticker":
		paper.tickerChannel <- kraken.NewTicker(raw).Data
	case "book":
		paper.bookChannel <- kraken.NewBook(raw).Data
	case "trade":
		paper.tradeChannel <- kraken.NewTrade(raw).Data
	case "balances":
		paper.balancesChannel <- kraken.NewBalance(raw)
	case "executions":
		paper.executionsChannel <- kraken.NewExecution(raw)
	case "instrument":
		paper.instrumentChannel <- kraken.NewInstrumentData(raw)
	case "add_order":
		paper.orderChannel <- kraken.NewOrderResponse(raw)
	}

	return nil
}

func (paper *Paper) Close() {
	paper.cancel()
}

func (paper *Paper) SubBalances() error {
	return paper.lifecycle.Balance("snapshot")
}

func (paper *Paper) SubExecutions() error {
	return nil
}

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
OpenOrders reports resting paper orders. Paper fills complete synchronously
inside Lifecycle.Place, so no order is ever left outstanding and boot reconcile
always sees an empty set.
*/
func (paper *Paper) OpenOrders() (map[string]spot.Order, error) {
	return map[string]spot.Order{}, nil
}

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

	return paper.lifecycle.Place(model, order.ReqID)
}

func (paper *Paper) execute(entity string, command ...string) (datura.Map[any], error) {
	input := []string{
		"paper",
	}

	input = append(input, command...)
	input = append(input, "--output", "json")

	cmd := exec.Command("kraken", input...)

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

	err = sonic.Unmarshal(stdout, &model)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to subscribe to "+entity,
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
