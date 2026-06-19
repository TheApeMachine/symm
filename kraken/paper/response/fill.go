package response

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/broker"
)

/*
FillSimulator prices paper orders from tree quotes and applies configured slippage.
*/
type FillSimulator struct {
	quotes *broker.QuoteCache
}

func NewFillSimulator(tree *dmt.Tree) *FillSimulator {
	return &FillSimulator{quotes: broker.NewQuoteCache(tree)}
}

func (simulator *FillSimulator) Preflight(wire map[string]any) error {
	if simulator == nil || simulator.quotes == nil {
		return nil
	}

	symbol := stringField(wire, "symbol")
	quote, ok := simulator.quotes.QuoteForSymbol(symbol)

	if !ok {
		return errMissingQuote(symbol)
	}

	return PreflightGates(preflightFromWire(wire, quote))
}

func (simulator *FillSimulator) Simulate(wire map[string]any) (FillNotice, error) {
	notice := fillNoticeFromWire(wire)
	quote, ok := simulator.quotes.QuoteForSymbol(notice.Symbol)

	if !ok {
		return FillNotice{}, errMissingQuote(notice.Symbol)
	}

	fill, err := SlippageFill(quote, notice.Side, notice.OrderQty)

	if err != nil {
		return FillNotice{}, err
	}

	price := ApplyExtraSlippageBps(
		fill.Price,
		notice.Side,
		viper.GetFloat64("trading.paper.slippage_bps"),
	)

	return FillNotice{
		Symbol:       notice.Symbol,
		Side:         notice.Side,
		OrderQty:     notice.OrderQty,
		ClOrdID:      notice.ClOrdID,
		OrderType:    notice.OrderType,
		OrderID:      uuid.NewString(),
		Price:        price,
		Reason:       "paper_fill",
		LiquidityInd: "t",
	}, nil
}

func (simulator *FillSimulator) LatencyDelay() time.Duration {
	if viper.GetBool("trading.paper.deterministic") {
		return 0
	}

	return EffectiveNetworkLatency()
}

func symbolParts(symbol string) (base, quote string) {
	parts := strings.Split(symbol, "/")

	if len(parts) != 2 {
		return "", ""
	}

	return strings.ToUpper(parts[0]), strings.ToUpper(parts[1])
}

type quoteError string

func (err quoteError) Error() string { return string(err) }

func errMissingQuote(symbol string) error {
	return quoteError("paper: missing quote for " + symbol)
}
