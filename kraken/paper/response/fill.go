package response

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

/*
FillSimulator prices paper orders from tree ingest artifacts and applies slippage.
*/
type FillSimulator struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	tree    *dmt.Tree
	latency *Latency
}

/*
NewFillSimulator wires paper fill simulation against the shared ingest tree.
*/
func NewFillSimulator(ctx context.Context, tree *dmt.Tree) *FillSimulator {
	ctx, cancel := context.WithCancel(ctx)

	latency := NewLatency().Load(
		viper.GetString("trading.paper.latency_profile"),
	)

	fillSimulator := &FillSimulator{
		ctx:     ctx,
		cancel:  cancel,
		tree:    tree,
		latency: latency,
	}

	if latency != nil && latency.Error() != nil {
		fillSimulator.err = latency.Error()
	}

	return fillSimulator
}

/*
Preflight rejects orders when quote quality or projected slippage is unacceptable.
*/
func (fillSimulator *FillSimulator) Preflight(order *datura.Artifact) error {
	if fillSimulator == nil || fillSimulator.tree == nil || order == nil {
		return nil
	}

	symbol := datura.Peek[string](order, "symbol")
	quote, quoteOK := fillSimulator.quoteForSymbol(symbol)

	if !quoteOK {
		return fmt.Errorf("paper: missing quote for %s", symbol)
	}

	defer quote.Release()

	return fillSimulator.preflightGatesAt(order, quote, time.Now().UTC())
}

/*
Simulate produces a paper fill artifact from the latest tree quote and slippage model.
*/
func (fillSimulator *FillSimulator) Simulate(
	order *datura.Artifact,
	orderID string,
) (*datura.Artifact, error) {
	if order == nil {
		return nil, fmt.Errorf("paper fill: order artifact is nil")
	}

	symbol := datura.Peek[string](order, "symbol")
	quote, quoteOK := fillSimulator.quoteForSymbol(symbol)

	if !quoteOK {
		return nil, fmt.Errorf("paper: missing quote for %s", symbol)
	}

	defer quote.Release()

	return fillSimulator.fillFromOrder(order, quote, orderID)
}

func (fillSimulator *FillSimulator) fillFromOrder(
	order *datura.Artifact,
	quote *datura.Artifact,
	orderID string,
) (*datura.Artifact, error) {
	side := datura.Peek[string](order, "side")
	qty := datura.Peek[float64](order, "order_qty")

	fill, err := fillSimulator.slippageFill(quote, side, qty)

	if err != nil {
		return nil, err
	}

	defer fill.Release()

	price := fillSimulator.applyExtraSlippageBps(
		datura.Peek[float64](fill, "price"),
		side,
		viper.GetFloat64("trading.paper.slippage_bps"),
	)

	notice := datura.Acquire("paper", datura.Artifact_Type_json)
	notice.WithRole("fill")
	notice.WithScope(datura.Peek[string](order, "symbol"))
	notice.WithPayload(datura.Map[any]{
		"symbol":        datura.Peek[string](order, "symbol"),
		"side":          side,
		"order_qty":     qty,
		"cl_ord_id":     datura.Peek[string](order, "cl_ord_id"),
		"order_type":    datura.Peek[string](order, "order_type"),
		"order_id":      orderID,
		"exec_id":       orderID,
		"last_price":    price,
		"avg_price":     price,
		"order_status":  "filled",
		"exec_type":     "trade",
		"liquidity_ind": "t",
	}.Marshal())

	return notice, nil
}

/*
Error returns the simulator's terminal error.
*/
func (fillSimulator *FillSimulator) Error() error {
	if fillSimulator.err != nil {
		return fillSimulator.err
	}

	if fillSimulator.latency != nil {
		return fillSimulator.latency.Error()
	}

	return nil
}

/*
Close shuts down the simulator context.
*/
func (fillSimulator *FillSimulator) Close() error {
	fillSimulator.cancel()

	return nil
}

func (fillSimulator *FillSimulator) quoteForSymbol(symbol string) (*datura.Artifact, bool) {
	if fillSimulator == nil || fillSimulator.tree == nil || symbol == "" {
		return nil, false
	}

	ticker, tickerOK := fillSimulator.latestIngest("ticker", symbol)

	if !tickerOK {
		return nil, false
	}

	defer ticker.Release()

	last := fillSimulator.payloadNumber(ticker, "data", 0, "last")
	bid := fillSimulator.payloadNumber(ticker, "data", 0, "bid")
	ask := fillSimulator.payloadNumber(ticker, "data", 0, "ask")

	if last <= 0 && (bid <= 0 || ask <= 0) {
		return nil, false
	}

	quote := datura.Acquire("paper", datura.Artifact_Type_json)
	quote.WithRole("quote")
	quote.WithScope(symbol)

	body := datura.Map[any]{
		"last":       last,
		"bid":        bid,
		"ask":        ask,
		"updated_at": time.Now().UTC().Format(time.RFC3339Nano),
	}

	quote.WithPayload(body.Marshal())

	book, bookOK := fillSimulator.latestIngest("book", symbol)

	if bookOK {
		fillSimulator.attachBookDepth(quote, book)
		book.Release()
	}

	return quote, true
}

func (fillSimulator *FillSimulator) payloadNumber(
	artifact *datura.Artifact,
	path ...any,
) float64 {
	if artifact == nil {
		return 0
	}

	if value := datura.Peek[float64](artifact, path...); value != 0 {
		return value
	}

	if value := datura.Peek[int64](artifact, path...); value != 0 {
		return float64(value)
	}

	raw := datura.Peek[string](artifact, path...)

	if raw == "" {
		return 0
	}

	parsed, err := strconv.ParseFloat(raw, 64)

	if err != nil {
		return 0
	}

	return parsed
}

func (fillSimulator *FillSimulator) latestIngest(role, scope string) (*datura.Artifact, bool) {
	prefix := role + "/" + scope
	var latest *datura.Artifact

	for candidate := range fillSimulator.tree.Seek([]byte(prefix)) {
		if latest != nil {
			latest.Release()
		}

		latest = candidate
	}

	if latest == nil {
		return nil, false
	}

	return latest, true
}

func (fillSimulator *FillSimulator) attachBookDepth(
	quote *datura.Artifact,
	book *datura.Artifact,
) {
	if quote == nil || book == nil {
		return
	}

	fillSimulator.copyDepthSide(quote, book, "bids")
	fillSimulator.copyDepthSide(quote, book, "asks")
}

func (fillSimulator *FillSimulator) copyDepthSide(
	quote *datura.Artifact,
	book *datura.Artifact,
	side string,
) {
	for index := 0; index < 256; index++ {
		price := fillSimulator.payloadNumber(book, "data", 0, side, index, "price")
		qty := fillSimulator.payloadNumber(book, "data", 0, side, index, "qty")

		if price <= 0 {
			price = fillSimulator.payloadNumber(book, "data", 0, side, index, 0)
			qty = fillSimulator.payloadNumber(book, "data", 0, side, index, 1)
		}

		if price <= 0 {
			break
		}

		if qty <= 0 {
			continue
		}

		quote.Poke(price, side, index, 0)
		quote.Poke(qty, side, index, 1)
	}
}
