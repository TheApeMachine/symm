package response

import (
	"context"
	"fmt"
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
	now     func() time.Time
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
		now:     time.Now,
	}

	if latency != nil && latency.Error() != nil {
		fillSimulator.err = latency.Error()
	}

	return fillSimulator
}

func (fillSimulator *FillSimulator) SetClock(clock func() time.Time) {
	if fillSimulator == nil {
		return
	}
	if clock == nil {
		fillSimulator.now = time.Now
		return
	}
	fillSimulator.now = clock
}

func (fillSimulator *FillSimulator) currentTime() time.Time {
	if fillSimulator == nil || fillSimulator.now == nil {
		return time.Now().UTC()
	}

	return fillSimulator.now().UTC()
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
		return preflightReject{
			code:    "stale_quote",
			message: fmt.Sprintf("paper: missing quote for %s", symbol),
		}
	}

	defer quote.Release()

	return fillSimulator.preflightGatesAt(order, quote, fillSimulator.currentTime())
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
		"setup_key":     datura.Peek[string](order, "setup_key"),
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
