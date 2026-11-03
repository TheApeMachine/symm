package network

import (
	"context"
	"encoding/json"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/system"
	"golang.design/x/lockfree/lf"
)

var virtualREST = map[string]string{
	"/0/private/Balance":      "balances",
	"/0/private/TradeBalance": "balances",
}

var virtualType = map[string]func(map[string]any) json.Marshaler{
	"balances":   func(data map[string]any) json.Marshaler { return market.NewBalance(data) },
	"executions": func(data map[string]any) json.Marshaler { return market.NewExecution(data) },
	"add_order":  func(data map[string]any) json.Marshaler { return market.NewExecution(data) },
}

/*
Virtual is an in-process private Client so the virtual tier speaks the same
Read/Write/Post surface as paper and live.
*/
type Virtual struct {
	ctx    context.Context
	cancel context.CancelFunc
	queue  *lf.Queue[[]byte]
	notify chan struct{}
	wallet map[string]any
}

func NewVirtual(ctx context.Context) *Virtual {
	ctx, cancel := context.WithCancel(ctx)
	quote := system.Cfg.Market.QuoteCurrency
	cash := decimal.NewFromInt(int64(system.Cfg.Market.Balance))

	return &Virtual{
		ctx:    ctx,
		cancel: cancel,
		queue:  lf.NewQueue[[]byte](),
		notify: make(chan struct{}, 1),
		wallet: map[string]any{
			"quote": quote,
			"assets": []*market.Asset{
				market.NewAsset(nil, quote, cash, decimal.Zero, cash),
			},
		},
	}
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

func (virtual *Virtual) Write(message []byte) error {
	request := map[string]any{}

	if err := sonic.Unmarshal(message, &request); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "failed to unmarshal virtual request", err,
		))
	}

	method, _ := request["method"].(string)
	entity := method

	if method == "subscribe" {
		params, _ := request["params"].(map[string]any)
		entity, _ = params["channel"].(string)
	}

	build := virtualType[entity]

	if build == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[network.Virtual.Write] no adapter", nil,
		), "entity", entity)
	}

	model := virtual.wallet

	if entity != "balances" {
		model = map[string]any{"executions": []*market.Execution{}}
	}

	buf, err := build(model).MarshalJSON()

	if err != nil {
		return errnie.Error(err)
	}

	virtual.queue.Enqueue(buf)

	select {
	case virtual.notify <- struct{}{}:
	default:
	}

	return nil
}

func (virtual *Virtual) Post(url string, body any) ([]byte, error) {
	_ = body

	entity, ok := virtualREST[url]

	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[network.Virtual.Post] unsupported path", nil,
		), "url", url)
	}

	return virtualType[entity](virtual.wallet).MarshalJSON()
}

func (virtual *Virtual) Watch(
	url string,
	body any,
	callback func(json.Unmarshaler),
) {
	go func() {
		for {
			select {
			case <-virtual.ctx.Done():
				return
			default:
			}

			response, err := virtual.Post(url, body)

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
