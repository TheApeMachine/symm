package response

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
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

func (simulator *FillSimulator) Simulate(params trading.AddParams) (FillNotice, error) {
	quote, ok := simulator.quotes.QuoteForSymbol(params.Symbol)

	if !ok {
		return FillNotice{}, errMissingQuote(params.Symbol)
	}

	fill, err := broker.SlippageFill(quote, params.Side, params.OrderQty)

	if err != nil {
		return FillNotice{}, err
	}

	price := broker.ApplyExtraSlippageBps(
		fill.Price,
		params.Side,
		viper.GetFloat64("trading.paper.slippage_bps"),
	)

	return FillNotice{
		Params:       params,
		OrderID:      uuid.NewString(),
		Price:        price,
		Fee:          0,
		Reason:       "paper_fill",
		LiquidityInd: "t",
		Maker:        false,
		Partial:      false,
	}, nil
}

func (simulator *FillSimulator) LatencyDelay() time.Duration {
	if viper.GetBool("trading.paper.deterministic") {
		return 0
	}

	return broker.EffectiveNetworkLatency()
}

func fillExecution(notice FillNotice) user.Execution {
	now := time.Now().UTC().Format(time.RFC3339Nano)

	return user.Execution{
		OrderID:      notice.OrderID,
		ClOrdID:      notice.Params.ClOrdID,
		Symbol:       notice.Params.Symbol,
		Side:         string(notice.Params.Side),
		OrderType:    string(notice.Params.OrderType),
		OrderQty:     notice.Params.OrderQty,
		OrderStatus:  "filled",
		ExecType:     "trade",
		ExecID:       notice.OrderID,
		LastQty:      notice.Params.OrderQty,
		LastPrice:    notice.Price,
		AvgPrice:     notice.Price,
		CumQty:       notice.Params.OrderQty,
		LiquidityInd: notice.LiquidityInd,
		Timestamp:    now,
	}
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
