package network

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
	"golang.design/x/lockfree/lf"
)

var virtualREST = map[string]string{
	"/0/private/Balance":      "balances",
	"/0/private/TradeBalance": "balances",
}

/*
Fills prices virtual market orders from the live book.
*/
type Fills interface {
	FillAsk(symbol string, qty *decimal.Decimal) *decimal.Decimal
	FillBid(symbol string, qty *decimal.Decimal) *decimal.Decimal
}

/*
QuoteSettle applies desk fee math to a fill's gross quote notional.
Buy returns fee-inclusive debit; sell returns fee-net credit. feeAmt is the
taker dollars. Wired from broker via Price.afterFee — Virtual does not own fees.
*/
type QuoteSettle func(symbol string, gross decimal.Decimal, buy bool) (cash, feeAmt decimal.Decimal)

/*
Virtual is an in-process private Client so the virtual tier speaks the same
Read/Write/Post surface as paper and live.
*/
type Virtual struct {
	ctx    context.Context
	cancel context.CancelFunc
	queue  *lf.Queue[[]byte]
	notify chan struct{}
	mu     sync.Mutex
	wallet map[string]any
	fills  Fills
	settle QuoteSettle
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

/*
Close cancels the virtual private transport so blocked Reads return.
*/
func (virtual *Virtual) Close() error {
	virtual.cancel()
	return nil
}

/*
UseFills wires the book pricer used by add_order fills.
*/
func (virtual *Virtual) UseFills(fills Fills) {
	virtual.mu.Lock()
	virtual.fills = fills
	virtual.mu.Unlock()
}

/*
UseQuoteSettle wires fee-adjusted quote cash (broker Price.afterFee).
*/
func (virtual *Virtual) UseQuoteSettle(settle QuoteSettle) {
	virtual.mu.Lock()
	virtual.settle = settle
	virtual.mu.Unlock()
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

	switch method {
	case "subscribe":
		return virtual.subscribe(request)
	case "add_order":
		return virtual.addOrder(request)
	default:
		return errnie.Error(errnie.Err(
			errnie.Validation, "[network.Virtual.Write] no adapter", nil,
		), "entity", method)
	}
}

func (virtual *Virtual) subscribe(request map[string]any) error {
	params, _ := request["params"].(map[string]any)
	channel, _ := params["channel"].(string)

	switch channel {
	case "balances":
		return virtual.enqueueBalances()
	case "executions":
		return virtual.enqueue(&kraken.Execution{
			Channel: "executions",
			Type:    "snapshot",
			Data:    make([]kraken.ExecutionData, 0),
		})
	default:
		return errnie.Error(errnie.Err(
			errnie.Validation, "[network.Virtual.Write] no adapter", nil,
		), "entity", channel)
	}
}

func (virtual *Virtual) addOrder(request map[string]any) error {
	params, _ := request["params"].(map[string]any)

	if params == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[network.Virtual.addOrder] missing params", nil,
		))
	}

	symbol, _ := params["symbol"].(string)
	side, _ := params["side"].(string)
	qtyRaw, _ := params["order_qty"].(string)
	clOrdID, _ := params["cl_ord_id"].(string)
	side = strings.ToLower(side)

	if symbol == "" || (side != "buy" && side != "sell") || qtyRaw == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[network.Virtual.addOrder] bad params", nil,
		), "symbol", symbol, "side", side)
	}

	qty, err := decimal.NewFromString(qtyRaw)

	if err != nil || qty.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[network.Virtual.addOrder] bad order_qty", err,
		), "order_qty", qtyRaw)
	}

	virtual.mu.Lock()
	fills := virtual.fills
	settle := virtual.settle
	virtual.mu.Unlock()

	if fills == nil {
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable, "[network.Virtual.addOrder] fills not wired", nil,
		))
	}

	var px *decimal.Decimal

	if side == "buy" {
		px = fills.FillAsk(symbol, &qty)
	} else {
		px = fills.FillBid(symbol, &qty)
	}

	if px == nil || px.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.NotFound, "[network.Virtual.addOrder] no book price", nil,
		), "symbol", symbol, "side", side)
	}

	gross := qty.Mul(*px)
	cash := gross
	feeAmt := decimal.Zero

	if settle != nil {
		cash, feeAmt = settle(symbol, gross, side == "buy")
	}

	now := time.Now().UTC()
	orderID := "V-" + uuid.NewString()

	if err := virtual.applyFill(side, symbol, qty, cash); err != nil {
		return errnie.Error(err)
	}

	frame := &kraken.Execution{
		Channel: "executions",
		Type:    "update",
		Data: []kraken.ExecutionData{{
			OrderID:       orderID,
			ClientOrderID: clOrdID,
			ExecID:        orderID + "-1",
			ExecType:      "trade",
			Symbol:        symbol,
			Side:          side,
			LastQty:       decimalPtr(qty),
			LastPrice:     decimalPtr(*px),
			Cost:          decimalPtr(gross),
			OrderType:     "market",
			Timestamp:     now,
			OrderStatus:   "filled",
			CumQty:        decimalPtr(qty),
			CumCost:       decimalPtr(gross),
			AvgPrice:      decimalPtr(*px),
			FeeUsdEquiv:   decimalPtr(feeAmt),
		}},
	}

	if err := virtual.enqueue(frame); err != nil {
		return errnie.Error(err)
	}

	return virtual.enqueueBalances()
}

func (virtual *Virtual) applyFill(
	side, symbol string,
	qty, cost decimal.Decimal,
) error {
	virtual.mu.Lock()
	defer virtual.mu.Unlock()

	quote, _ := virtual.wallet["quote"].(string)

	if quote == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[network.Virtual.applyFill] missing quote", nil,
		))
	}

	base := symbol

	if i := strings.IndexByte(symbol, '/'); i > 0 {
		base = symbol[:i]
	}

	cash := virtual.asset(quote)
	lot := virtual.asset(base)

	if side == "buy" {
		if cash.Capital.Cmp(cost) < 0 {
			// Soft denial — Tier maps this to EntryDenial; do not ERROR-log.
			return errnie.Err(
				errnie.NotAcceptable,
				"[network.Virtual.applyFill] insufficient cash",
				nil,
			).With("need", cost.String(), "have", cash.Capital.String())
		}

		cash.Capital = cash.Capital.Sub(cost)
		cash.Equity = cash.Capital
		lot.Capital = lot.Capital.Add(qty)
		lot.Equity = lot.Capital
	} else {
		if lot.Capital.Cmp(qty) < 0 {
			return errnie.Err(
				errnie.NotAcceptable,
				"[network.Virtual.applyFill] insufficient base",
				nil,
			).With("need", qty.String(), "have", lot.Capital.String())
		}

		lot.Capital = lot.Capital.Sub(qty)
		lot.Equity = lot.Capital
		cash.Capital = cash.Capital.Add(cost)
		cash.Equity = cash.Capital
	}

	return nil
}

/*
Restock credits base for an S3-restored open lot and debits quote by cost so
Allocate does not treat starting cash as still fully free.
*/
func (virtual *Virtual) Restock(symbol string, qty, cost decimal.Decimal) {
	if qty.Sign() <= 0 {
		return
	}

	virtual.mu.Lock()
	defer virtual.mu.Unlock()

	base := symbol

	if i := strings.IndexByte(symbol, '/'); i > 0 {
		base = symbol[:i]
	}

	lot := virtual.asset(base)
	lot.Capital = lot.Capital.Add(qty)
	lot.Equity = lot.Capital

	if cost.Sign() <= 0 {
		return
	}

	quote, _ := virtual.wallet["quote"].(string)

	if quote == "" {
		return
	}

	cash := virtual.asset(quote)

	if cash.Capital.Cmp(cost) < 0 {
		cost = cash.Capital
	}

	cash.Capital = cash.Capital.Sub(cost)
	cash.Equity = cash.Capital
}

func (virtual *Virtual) asset(symbol string) *market.Asset {
	assets, _ := virtual.wallet["assets"].([]*market.Asset)

	for _, asset := range assets {
		if asset != nil && asset.Symbol == symbol {
			return asset
		}
	}

	out := market.NewAsset(nil, symbol, decimal.Zero, decimal.Zero, decimal.Zero)
	virtual.wallet["assets"] = append(assets, out)

	return out
}

func (virtual *Virtual) enqueueBalances() error {
	out := kraken.NewBalance()
	out.Channel = "balances"

	virtual.mu.Lock()
	book := market.NewBalance(virtual.wallet)
	out.FromMarket(book)
	virtual.mu.Unlock()

	return virtual.enqueue(out)
}

func (virtual *Virtual) enqueue(frame json.Marshaler) error {
	buf, err := frame.MarshalJSON()

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

	if entity != "balances" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[network.Virtual.Post] unsupported entity", nil,
		), "entity", entity)
	}

	out := kraken.NewBalance()
	out.Channel = "balances"

	virtual.mu.Lock()
	book := market.NewBalance(virtual.wallet)
	out.FromMarket(book)
	virtual.mu.Unlock()

	return out.MarshalJSON()
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

func decimalPtr(value decimal.Decimal) *decimal.Decimal {
	out := value
	return &out
}
