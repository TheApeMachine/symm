package execution

import (
	"embed"
	"iter"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/tests"
)

//go:embed fixtures/*.json
var fixtureFiles embed.FS

/*
Options parameterize one Kraken execution update frame.
*/
type Options struct {
	OrderID     string
	ExecID      string
	Symbol      string
	Side        string
	LastQty     string
	LastPrice   string
	Cost        string
	OrderStatus string
	CumQty      string
	CumCost     string
	AvgPrice    string
	FeeUsdEquiv string
	Timestamp   string
}

/*
BuyFill returns the cumulative buy fill used by broker execution tests.
*/
func BuyFill() Options {
	return Options{
		OrderID: "order-1", ExecID: "fill-2", Symbol: "BTC/USD", Side: "buy",
		LastQty: "1", LastPrice: "110", Cost: "110", OrderStatus: "filled",
		CumQty: "2", CumCost: "210", AvgPrice: "105",
		Timestamp: "2026-07-14T10:00:00Z",
	}
}

/*
ExitFill returns the final sell fill used by broker close tests.
*/
func ExitFill() Options {
	return Options{
		OrderID: "exit-1", ExecID: "exit-fill", Symbol: "BTC/USD", Side: "sell",
		LastQty: "1", LastPrice: "110", Cost: "110", OrderStatus: "filled",
		CumQty: "1", CumCost: "110", AvgPrice: "110", FeeUsdEquiv: "1",
		Timestamp: "2026-07-14T10:00:00Z",
	}
}

/*
ReduceFill returns the partial sell fill used by broker reduction tests.
*/
func ReduceFill() Options {
	return Options{
		OrderID: "reduce-1", ExecID: "reduce-fill", Symbol: "BTC/USD", Side: "sell",
		LastQty: "1", LastPrice: "110", Cost: "110", OrderStatus: "filled",
		CumQty: "1", CumCost: "110", AvgPrice: "110", FeeUsdEquiv: "1",
		Timestamp: "2026-07-14T10:00:00Z",
	}
}

/*
Frame builds one execution payload from explicit options.
*/
func Frame(options Options) []byte {
	raw, err := fixtureFiles.ReadFile("fixtures/fill.json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "execution fixture load failed", err))
	}

	var payload map[string]any

	if err := sonic.Unmarshal(raw, &payload); err != nil {
		panic(errnie.Err(errnie.Validation, "execution fixture decode failed", err))
	}

	rows, ok := payload["data"].([]any)

	if !ok || len(rows) == 0 {
		panic(errnie.Err(errnie.Validation, "execution fixture data missing", nil))
	}

	row, ok := rows[0].(map[string]any)

	if !ok {
		panic(errnie.Err(errnie.Validation, "execution fixture row missing", nil))
	}

	row["order_id"] = options.OrderID
	row["exec_id"] = options.ExecID
	row["symbol"] = options.Symbol
	row["side"] = options.Side
	row["last_qty"] = options.LastQty
	row["last_price"] = options.LastPrice
	row["cost"] = options.Cost
	row["order_status"] = options.OrderStatus
	row["cum_qty"] = options.CumQty
	row["cum_cost"] = options.CumCost
	row["avg_price"] = options.AvgPrice

	if options.FeeUsdEquiv != "" {
		row["fee_usd_equiv"] = options.FeeUsdEquiv
	}

	if options.Timestamp != "" {
		row["timestamp"] = options.Timestamp
	}

	return tests.MarshalFrame(payload)
}

/*
Fixture replays one ordered execution sequence for broker position tests.
*/
type Fixture struct {
	payloads [][]byte
}

/*
NewFixture builds an execution sequence from explicit options.
*/
func NewFixture(options ...Options) *Fixture {
	payloads := make([][]byte, len(options))

	for index, option := range options {
		payloads[index] = Frame(option)
	}

	return &Fixture{payloads: payloads}
}

func (fixture *Fixture) Generate() iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		for _, payload := range fixture.payloads {
			if !yield(payload) {
				return
			}
		}
	}
}

func (fixture *Fixture) Frames() iter.Seq[tests.Frame] {
	return tests.FrameSequence(fixture.Generate())
}
