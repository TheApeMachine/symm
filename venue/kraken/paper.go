package kraken

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/venue"
	"golang.design/x/lockfree/lf"
)

var commandMap = map[string][]string{
	"balances":     {"balance", "--verbose"},
	"executions":   {"history", "--verbose"},
	"history":      {"history", "--verbose"},
	"tradebalance": {"status", "--verbose"},
}

var typeMap = map[string]func(model map[string]any) json.Marshaler{
	"balances":     NewBalanceFromMap,
	"executions":   NewExecutionsFromHistoryMap,
	"add_order":    NewOrderResponseFromMap,
	"history":      NewTradesHistoryFromMap,
	"tradebalance": NewTradeBalanceEnvelopeFromMap,
}

var restMap = map[string]string{
	"/0/private/Balance":      "balances",
	"/0/private/TradeBalance": "tradebalance",
}

/*
Paper is the simulated private spot websocket. It shells out to the native
`kraken paper` CLI so the venue ledger owns balances and fills. Same Read/Write
envelopes as Live — the private ingress cannot tell them apart.
*/
type Paper struct {
	ctx       context.Context
	cancel    context.CancelFunc
	simulator *venue.Simulator
	queue     *lf.Queue[[]byte]
	notify    chan struct{}
}

func NewPaper(ctx context.Context, simulator *venue.Simulator) *Paper {
	if simulator == nil {
		simulator = venue.NewSimulator()
	}

	ctx, cancel := context.WithCancel(ctx)

	return &Paper{
		ctx:       ctx,
		cancel:    cancel,
		simulator: simulator,
		queue:     lf.NewQueue[[]byte](),
		notify:    make(chan struct{}, 1),
	}
}

func (paper *Paper) Read(keys ...string) ([]byte, error) {
	for {
		if buf, ok := paper.queue.Dequeue(); ok {
			return buf, nil
		}

		select {
		case <-paper.ctx.Done():
			return nil, errnie.Error(paper.ctx.Err())
		case <-paper.notify:
		}
	}
}

/*
Write routes the same subscription and order envelopes used by live through
the paper CLI under simulator latency, then queues the response frame.
*/
func (paper *Paper) Write(message []byte) error {
	request := map[string]any{}

	if err := sonic.Unmarshal(message, &request); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "failed to unmarshal paper request", err,
		))
	}

	method, _ := request["method"].(string)
	entity, command, err := paper.route(method, request)

	if err != nil {
		return err
	}

	var model map[string]any

	paper.simulator.Do(venue.WEBSOCKET, func() {
		model, err = paper.execute(entity, command...)
	})

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal, "failed to execute paper command", err,
		))
	}

	build := typeMap[entity]

	if build == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[kraken.Paper.Write] no adapter for entity", nil,
		), "entity", entity)
	}

	buf, err := build(model).MarshalJSON()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal, "failed to marshal paper response", err,
		))
	}

	paper.queue.Enqueue(buf)

	select {
	case paper.notify <- struct{}{}:
	default:
	}

	return nil
}

func (paper *Paper) Watch(
	url string,
	body any,
	callback func(json.Unmarshaler),
) {
	go func() {
		for {
			select {
			case <-paper.ctx.Done():
				return
			default:
			}

			response, err := paper.Post(url, body)

			if err != nil {
				continue
			}

			var data json.Unmarshaler

			if err := json.Unmarshal(response, &data); err != nil {
				continue
			}

			callback(data)
		}
	}()
}

func (paper *Paper) route(
	method string, request map[string]any,
) (entity string, command []string, err error) {
	switch method {
	case "subscribe":
		params, _ := request["params"].(map[string]any)
		channel, _ := params["channel"].(string)
		command, ok := commandMap[channel]

		if !ok {
			return "", nil, errnie.Error(errnie.Err(
				errnie.Validation, "[kraken.Paper.Write] invalid paper channel", nil,
			), "channel", channel)
		}

		return channel, append([]string(nil), command...), nil

	case "add_order":
		params, _ := request["params"].(map[string]any)
		return "add_order", orderCommand(params), nil

	default:
		return "", nil, errnie.Error(errnie.Err(
			errnie.Validation, "[kraken.Paper.Write] invalid paper method", nil,
		), "method", method)
	}
}

func orderCommand(params map[string]any) []string {
	side, _ := params["side"].(string)
	symbol, _ := params["symbol"].(string)
	qty, _ := params["order_qty"].(string)
	command := []string{side, symbol, qty}

	if orderType, _ := params["order_type"].(string); orderType == "limit" {
		if price, _ := params["limit_price"].(string); price != "" {
			command = append(command, "--type", "limit", "--price", price)
		}
	}

	if clOrdID, _ := params["cl_ord_id"].(string); clOrdID != "" {
		command = append(command, "--cl-ord-id", clOrdID)
	}

	return command
}

func (paper *Paper) execute(entity string, command ...string) (map[string]any, error) {
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

	model := map[string]any{}

	if err := sonic.Unmarshal(stdout, &model); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[kraken.Paper.execute] failed to unmarshal response",
			err,
		), "entity", entity)
	}

	if errCategory, ok := model["error"].(string); ok && errCategory != "" {
		message, _ := model["message"].(string)

		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[kraken.Paper.execute] paper command rejected",
			nil,
		), "message", message)
	}

	return model, nil
}

/*
Post serves the same private REST paths as Live, via the paper CLI.
*/
func (paper *Paper) Post(url string, body any) ([]byte, error) {
	_ = body

	entity, ok := restMap[url]

	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[kraken.Paper.Post] unsupported path", nil,
		), "url", url)
	}

	model, err := paper.execute(entity, commandMap[entity]...)

	if err != nil {
		return nil, errnie.Error(err)
	}

	return typeMap[entity](model).MarshalJSON()
}
