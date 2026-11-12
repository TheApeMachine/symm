package network

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
	"github.com/theapemachine/symm/venue/kraken"
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

var commandMap = map[string][]string{
	"balances":     {"balance", "--verbose"},
	"executions":   {"history", "--verbose"},
	"history":      {"history", "--verbose"},
	"tradebalance": {"status", "--verbose"},
}

var typeMap = map[string]func(model map[string]any) json.Marshaler{
	"balances":     kraken.NewBalanceFromMap,
	"executions":   kraken.NewExecutionFromMap,
	"add_order":    kraken.NewOrderFromMap,
	"history":      kraken.NewTradesHistoryFromMap,
	"tradebalance": kraken.NewTradeBalanceFromMap,
}

var restMap = map[string]string{
	"/0/private/Balance":      "balances",
	"/0/private/TradeBalance": "tradebalance",
}

/*
Virtual is the simulated private spot websocket. It shells out to the native
`kraken paper` CLI so the venue ledger owns balances and fills. Same Read/Write
envelopes as Live — the private ingress cannot tell them apart.
*/
type Virtual struct {
	ctx    context.Context
	cancel context.CancelFunc
	queue  *lf.Queue[[]byte]
	notify chan struct{}
}

func NewVirtual(ctx context.Context) *Virtual {
	ctx, cancel := context.WithCancel(ctx)

	return &Virtual{
		ctx:    ctx,
		cancel: cancel,
		queue:  lf.NewQueue[[]byte](),
		notify: make(chan struct{}, 1),
	}
}

/*
Close cancels the virtual private transport so blocked Reads return.
*/
func (virtual *Virtual) Close() error {
	virtual.cancel()
	return nil
}

func (virtual *Virtual) Read(keys ...string) ([]byte, error) {
	for {
		if buf, ok := virtual.queue.Dequeue(); ok {
			return buf, nil
		}

		select {
		case <-virtual.ctx.Done():
			return nil, errnie.Error(virtual.ctx.Err())
		case <-virtual.notify:
		}
	}
}

/*
Write routes the same subscription and order envelopes used by live through
the paper CLI under simulator latency, then queues the response frame.
*/
func (virtual *Virtual) Write(message []byte) error {
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

		var req kraken.AddOrderRequest

		if err := sonic.Unmarshal(raw, &req); err != nil {
			return errnie.Error(err)
		}

		clOrdID = req.ClOrdId
		command = req.Command()
	}

	if command == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[kraken.Virtual.Write] no virtual command", nil,
		), "entity", entity)
	}

	model, err := virtual.execute(entity, append([]string(nil), command...)...)

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

	virtual.queue.Enqueue(buf)
	virtual.signal()

	if entity == "add_order" {
		virtual.enqueueFill(model, clOrdID)
	}

	return nil
}

func (virtual *Virtual) signal() {
	select {
	case virtual.notify <- struct{}{}:
	default:
	}
}

/*
enqueueFill queues the trade report behind the order ack so ingress can fold it.
*/
func (virtual *Virtual) enqueueFill(model map[string]any, clOrdID string) {
	exec, ok := kraken.NewExecutionFromMap(model).(*kraken.Execution)

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

	virtual.queue.Enqueue(raw)
	virtual.signal()
}

func (virtual *Virtual) Watch(
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
			case <-virtual.ctx.Done():
				return
			case <-ticker.C:
			}

			response, err := virtual.Post(url, body)

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

func (virtual *Virtual) execute(entity string, command ...string) (map[string]any, error) {
	input := []string{"paper"}
	input = append(input, command...)
	input = append(input, "--output", "json")

	cmd := exec.CommandContext(virtual.ctx, "kraken", input...)

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
			"kraken virtual "+entity+" command failed: "+details,
			err,
		))
	}

	model := map[string]any{}

	if err := sonic.Unmarshal(stdout, &model); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[kraken.Virtual.execute] failed to unmarshal response",
			err,
		), "entity", entity)
	}

	if errCategory, ok := model["error"].(string); ok && errCategory != "" {
		message, _ := model["message"].(string)

		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[kraken.Virtual.execute] virtual command rejected",
			nil,
		), "message", message)
	}

	return model, nil
}

/*
Post serves the same private REST paths as Live, via the paper CLI.
Balance polls return the venue Balance observation for Client.Watch.
*/
func (virtual *Virtual) Post(url string, body any) ([]byte, error) {
	_ = body

	entity, ok := restMap[RestPath(url)]

	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[kraken.Virtual.Post] unsupported path", nil,
		), "url", url)
	}

	model, err := virtual.execute(entity, commandMap[entity]...)

	if err != nil {
		return nil, errnie.Error(err)
	}

	return typeMap[entity](model).MarshalJSON()
}
