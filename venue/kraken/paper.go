package kraken

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"golang.design/x/lockfree/lf"
)

/*
RestPath returns the URL path for REST map lookups. Full URLs and bare paths
both resolve to the map keys.
*/
func RestPath(raw string) string {
	if parsed, err := url.Parse(raw); err == nil && parsed.Path != "" {
		return parsed.Path
	}

	return raw
}

/*
commandMap routes paper entity names to native kraken CLI argv for each private channel.
*/
var commandMap = map[string][]string{
	"balances":     {"balance", "--verbose"},
	"executions":   {"history", "--verbose"},
	"history":      {"history", "--verbose"},
	"tradebalance": {"status", "--verbose"},
}

/*
typeMap rebuilds live wire types from CLI JSON so venue.Read and broker desk see identical shapes.
*/
var typeMap = map[string]func(model map[string]any) json.Marshaler{
	"balances":     NewBalanceFromMap,
	"executions":   NewExecutionFromMap,
	"add_order":    NewOrderFromMap,
	"history":      NewTradesHistoryFromMap,
	"tradebalance": NewTradeBalanceFromMap,
}

/*
restMap keys private REST paths to paper entities for Watch/Post balance polls.
*/
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
	ctx    context.Context
	cancel context.CancelFunc
	queue  *lf.Queue[[]byte]
	notify chan struct{}
}

/*
NewPaper starts the lock-free queue transport network.Client uses for paper tier.
*/
func NewPaper(ctx context.Context) *Paper {
	ctx, cancel := context.WithCancel(ctx)

	return &Paper{
		ctx:    ctx,
		cancel: cancel,
		queue:  lf.NewQueue[[]byte](),
		notify: make(chan struct{}, 1),
	}
}

/*
Close cancels the paper private transport so blocked Reads return.
*/
func (paper *Paper) Close() error {
	paper.cancel()
	return nil
}

/*
Read blocks until a CLI response frame is queued or ctx is cancelled — same
contract as live websocket Read for broker ingress.
*/
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
	entity := method
	params, _ := request["params"].(map[string]any)
	var clOrdID string

	if method == "subscribe" {
		entity, _ = params["channel"].(string)
	}

	command := commandMap[entity]

	if entity == "add_order" {
		raw, err := sonic.Marshal(params)

		if err != nil {
			return errnie.Error(err)
		}

		var req AddOrderRequest

		if err := sonic.Unmarshal(raw, &req); err != nil {
			return errnie.Error(err)
		}

		clOrdID = req.ClOrdId
		command = req.Command()
	}

	if command == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[kraken.Paper.Write] no paper command", nil,
		), "entity", entity)
	}

	model, err := paper.execute(entity, append([]string(nil), command...)...)

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
	paper.signal()

	if entity == "add_order" {
		paper.enqueueFill(model, clOrdID)
	}

	return nil
}

/*
signal coalesces wakeups so one blocked Read drains the whole queue batch.
*/
func (paper *Paper) signal() {
	select {
	case paper.notify <- struct{}{}:
	default:
	}
}

/*
enqueueFill queues the trade report behind the order ack so ingress can fold it.
*/
func (paper *Paper) enqueueFill(model map[string]any, clOrdID string) {
	exec, ok := NewExecutionFromMap(model).(*Execution)

	if !ok || !exec.Tradable() {
		return
	}

	for i := range exec.Data {
		if exec.Data[i].ClientOrderID == "" {
			exec.Data[i].ClientOrderID = clOrdID
		}
	}

	raw, err := exec.MarshalJSON()

	if err != nil {
		errnie.Error(err)
		return
	}

	paper.queue.Enqueue(raw)
	paper.signal()
}

/*
Watch polls paper Post on an interval into dest — mirrors live REST Watch for Balance sync.
*/
func (paper *Paper) Watch(
	url string,
	body any,
	dest json.Unmarshaler,
) {
	if dest == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-paper.ctx.Done():
				return
			case <-ticker.C:
			}

			response, err := paper.Post(url, body)

			if err != nil {
				errnie.Error(err)
				continue
			}

			err = json.Unmarshal(response, dest)

			if err != nil {
				errnie.Error(err)
			}
		}
	}()
}

/*
paperCommandFunc abstracts CLI execution so tests can stub without forking kraken.
*/
type paperCommandFunc func(ctx context.Context, entity string, command ...string) (map[string]any, error)

/*
runPaperCommand is the production CLI backend; tests replace this hook.
*/
var runPaperCommand paperCommandFunc = execPaperCommand

/*
execPaperCommand runs kraken paper and returns JSON stdout as a generic map.
*/
func execPaperCommand(ctx context.Context, entity string, command ...string) (map[string]any, error) {
	input := []string{"paper"}
	input = append(input, command...)
	input = append(input, "--output", "json")

	cmd := exec.CommandContext(ctx, "kraken", input...)

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
execute forwards to runPaperCommand under paper ctx for cancellation on Close.
*/
func (paper *Paper) execute(entity string, command ...string) (map[string]any, error) {
	return runPaperCommand(paper.ctx, entity, command...)
}

/*
Post serves the same private REST paths as Live, via the paper CLI.
Balance polls return the venue Balance observation for Client.Watch.
*/
func (paper *Paper) Post(url string, body any) ([]byte, error) {
	_ = body

	entity, ok := restMap[RestPath(url)]

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
