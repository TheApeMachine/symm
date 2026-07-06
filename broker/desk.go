package broker

import (
	"context"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
)

const (
	channelTicker     = "ticker"
	channelBalances   = "balances"
	channelExecutions = "executions"
)

type Desk struct {
	ctx        context.Context
	cancel     context.CancelFunc
	channels   map[string]chan []byte
	public     websocket.Socket
	private    websocket.Private
	balance    *kraken.BalanceDataSlice
	executions []*kraken.ExecutionDataSlice
	UIForward  chan []byte
}

func NewDesk(
	ctx context.Context,
	public websocket.Socket,
	private websocket.Private,
) (*Desk, error) {
	ctx, cancel := context.WithCancel(ctx)

	if public == nil {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: public stream required",
			nil,
		))
	}

	if private == nil {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: private stream required",
			nil,
		))
	}

	return &Desk{
		ctx:     ctx,
		cancel:  cancel,
		public:  public,
		private: private,
		channels: map[string]chan []byte{
			channelTicker:     public.Observe(channelTicker),
			channelBalances:   private.Observe(channelBalances),
			channelExecutions: private.Observe(channelExecutions),
		},
	}, nil
}

func (desk *Desk) Ready() bool {
	return errnie.Require(map[string]any{
		"balance": desk.balance,
	}) == nil
}

/*
Run processes websocket and private frame streams until ctx closes.
*/
func (desk *Desk) Run() (err error) {
	for {
		select {
		case <-desk.ctx.Done():
			return nil
		case msg := <-desk.channels[channelBalances]:
			desk.balance = kraken.NewBalanceDataSlice(msg)
			desk.forward(desk.balance)
		case msg := <-desk.channels[channelExecutions]:
			slice := kraken.NewExecutionDataSlice(msg)
			desk.executions = append(desk.executions, slice)
			desk.forward(slice)
		case msg := <-desk.channels[channelTicker]:
			_ = msg
		}
	}
}

func (desk *Desk) forward(data any) {
	if desk.UIForward == nil || data == nil {
		return
	}

	buf, err := sonic.Marshal(data)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"desk: failed to serialize UI forward data",
			err,
		))

		return
	}

	select {
	case desk.UIForward <- buf:
	default:
	}
}

func (desk *Desk) Buy(symbol string, fraction float64, price float64) error {
	symbol = strings.TrimSpace(symbol)
	_, quote, ok := strings.Cut(symbol, "/")
	if !ok || strings.TrimSpace(quote) == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: buy symbol must include base and quote",
			nil,
		))
	}

	if fraction <= 0 || fraction > 1 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: buy fraction must be within the quote balance",
			nil,
		))
	}

	if price <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: buy price must be positive",
			nil,
		))
	}

	if desk.balance == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: balance required",
			nil,
		))
	}

	notional := 0.0
	for _, row := range *desk.balance {
		if strings.EqualFold(row.Asset, quote) {
			notional = row.Available * fraction
			break
		}
	}

	if notional <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: buy quote balance must be positive",
			nil,
		))
	}

	quantity := notional / price

	return desk.private.Submit(&kraken.Order{
		Method: "add_order",
		Params: kraken.LimitOrderParams{
			OrderType: "market",
			Side:      "buy",
			OrderQty:  quantity,
			Symbol:    symbol,
		},
		ReqID: int(time.Now().UnixNano()),
	})
}

func (desk *Desk) Sell(symbol string) error {
	base, _, ok := strings.Cut(strings.TrimSpace(symbol), "/")
	if !ok || strings.TrimSpace(base) == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: sell symbol must include base and quote",
			nil,
		))
	}

	if desk.balance == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: balance required",
			nil,
		))
	}

	quantity := 0.0
	for _, row := range *desk.balance {
		if strings.EqualFold(row.Asset, base) {
			quantity = row.Balance
			break
		}
	}

	if quantity <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: sell balance must be positive",
			nil,
		))
	}

	return desk.private.Submit(&kraken.Order{
		Method: "add_order",
		Params: kraken.LimitOrderParams{
			OrderType: "market",
			Side:      "sell",
			OrderQty:  quantity,
			Symbol:    strings.TrimSpace(symbol),
		},
		ReqID: int(time.Now().UnixNano()),
	})
}

func (desk *Desk) Close() error {
	desk.cancel()
	return nil
}
