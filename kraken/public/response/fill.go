package response

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
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
	symbol := datura.Peek[string](order, "symbol")
	side := datura.Peek[string](order, "side")
	orderType := datura.Peek[string](order, "order_type")
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

	rate, liquidity, feeSource, feeErr := fillSimulator.feeRate(symbol, orderType)

	if feeErr != nil {
		return nil, feeErr
	}

	fee := price * qty * rate
	feeCurrency := quoteAsset(symbol)

	notice := datura.Acquire("paper", datura.Artifact_Type_json)
	notice.WithRole("fill")
	notice.WithScope(symbol)
	notice.WithPayload(datura.Map[any]{
		"symbol":        symbol,
		"side":          side,
		"order_qty":     qty,
		"cl_ord_id":     datura.Peek[string](order, "cl_ord_id"),
		"order_type":    orderType,
		"order_id":      orderID,
		"exec_id":       orderID,
		"last_price":    price,
		"avg_price":     price,
		"fee":           fee,
		"fee_ccy":       feeCurrency,
		"fee_source":    feeSource,
		"order_status":  "filled",
		"exec_type":     "trade",
		"liquidity_ind": liquidity,
	}.Marshal())

	return notice, nil
}

/*
feeRate resolves the real Kraken maker/taker fee for the symbol from the
AssetPairs schedule stored in the tree. Resting limit orders pay the maker fee;
everything else pays the taker fee. When live AssetPairs metadata has not reached
the tree, paper mode can use configured conservative fee bps; the fill artifact
then carries fee_source=configured instead of assetpairs. The returned rate is a
fraction (0.0040 == 0.40%), alongside the Kraken liquidity indicator ("m" maker,
"t" taker).
*/
func (fillSimulator *FillSimulator) feeRate(
	symbol, orderType string,
) (float64, string, string, error) {
	if fillSimulator == nil || fillSimulator.tree == nil || symbol == "" {
		return 0, "", "", errnie.Error(errnie.Err(
			errnie.Validation, "paper fee: simulator, tree, or symbol missing", nil,
		))
	}

	schedule, ok := fillSimulator.latestIngest("assetpairs", symbol)

	if !ok {
		return configuredFeeRate(symbol, orderType)
	}

	defer schedule.Release()

	tierKey := "fees"
	liquidity := "t"

	if orderType == "limit" {
		tierKey = "fees_maker"
		liquidity = "m"
	}

	percent := fillSimulator.payloadNumber(schedule, tierKey, 0, 1)

	if percent <= 0 {
		return 0, "", "", errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("paper fee: non-positive %s for %s", tierKey, symbol),
			nil,
		))
	}

	return percent / 100.0, liquidity, "assetpairs", nil
}

func configuredFeeRate(symbol, orderType string) (float64, string, string, error) {
	liquidity := "t"
	key := "trading.paper.taker_fee_bps"

	if orderType == "limit" {
		liquidity = "m"
		key = "trading.paper.maker_fee_bps"
	}

	bps := viper.GetFloat64(key)
	if bps <= 0 {
		return 0, "", "", errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("paper fee: missing AssetPairs schedule and %s for %s", key, symbol),
			nil,
		))
	}

	return bps / 10_000.0, liquidity, "configured", nil
}

func quoteAsset(symbol string) string {
	parts := strings.Split(symbol, "/")

	if len(parts) != 2 {
		return ""
	}

	return strings.ToUpper(parts[1])
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
		"last": last,
		"bid":  bid,
		"ask":  ask,
	}
	if ticker.Timestamp() > 0 {
		body["updated_at"] = time.Unix(0, ticker.Timestamp()).UTC().Format(time.RFC3339Nano)
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
	var latest *datura.Artifact

	// Frames are keyed role-first with the timestamp ahead of the scope, so we
	// seek the role and keep the last artifact whose scope matches the symbol:
	// iteration is in key order, so the final match is the freshest.
	for candidate := range fillSimulator.tree.Seek([]byte(role + "/")) {
		candidateScope, _ := candidate.Scope()

		if candidateScope != scope {
			candidate.Release()

			continue
		}

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
