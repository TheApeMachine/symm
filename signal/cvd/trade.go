package cvd

import (
	"errors"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var errUnmeasurable = errors.New("cvd: trade requires a positive price and quantity")

type input struct {
	price    calculus.Constant
	quantity calculus.Constant
	buy      calculus.Constant
	sell     calculus.Constant
	midpoint calculus.Constant
	priorMid calculus.Constant
	quoted   calculus.Constant
	clock    temporal.Clock
	epoch    temporal.Clock
}

/*
Trade is the executed-flow measuring entity: one composition, the slots one
trade is written onto, and the quote source. It holds no accounting of its own.
*/
type Trade struct {
	mutex  sync.Mutex
	number *nomagique.Pipeline

	in     input
	symbol string
	at     time.Time

	quote   func(string) (*decimal.Decimal, *decimal.Decimal)
	priorMd map[string]float64
}

/*
NewTrade composes the executed-flow measurement as one Number pipeline.
*/
func NewTrade() *Trade {
	trade := &Trade{priorMd: make(map[string]float64)}

	keyFn := func() string { return trade.symbol }
	trade.in.clock.Key = keyFn
	trade.in.epoch.Key = keyFn

	in := &trade.in
	notional := &nmtypes.Product{A: &in.price, B: &in.quantity}

	buyQty := &calculus.Accumulator{
		Source: &calculus.Gate{Source: &in.quantity, When: &in.buy},
		Key:    keyFn,
	}
	sellQty := &calculus.Accumulator{
		Source: &calculus.Gate{Source: &in.quantity, When: &in.sell},
		Key:    keyFn,
	}

	buyCount := &calculus.Accumulator{
		Source: &calculus.Gate{Source: &calculus.Constant{Value: 1}, When: &in.buy},
		Key:    keyFn,
	}
	sellCount := &calculus.Accumulator{
		Source: &calculus.Gate{Source: &calculus.Constant{Value: 1}, When: &in.sell},
		Key:    keyFn,
	}

	buyNotional := &calculus.Accumulator{
		Source: &calculus.Gate{Source: notional, When: &in.buy},
		Key:    keyFn,
	}
	sellNotional := &calculus.Accumulator{
		Source: &calculus.Gate{Source: notional, When: &in.sell},
		Key:    keyFn,
	}

	tradeCount := &nmtypes.Sum{A: buyCount.Read(), B: sellCount.Read()}
	grossQty := &nmtypes.Sum{A: buyQty.Read(), B: sellQty.Read()}
	netQty := &calculus.Difference{Minuend: buyQty.Read(), Subtrahend: sellQty.Read()}
	grossNotional := &nmtypes.Sum{A: buyNotional.Read(), B: sellNotional.Read()}
	netNotional := &calculus.Difference{Minuend: buyNotional.Read(), Subtrahend: sellNotional.Read()}

	signedCountFraction := &calculus.Ratio{
		Numerator:   &calculus.Difference{Minuend: buyCount.Read(), Subtrahend: sellCount.Read()},
		Denominator: tradeCount,
	}
	meanTradeNotional := &calculus.Ratio{
		Numerator:   grossNotional,
		Denominator: tradeCount,
	}
	signedNetFraction := &calculus.Ratio{
		Numerator:   netNotional,
		Denominator: grossNotional,
	}

	elapsed := &calculus.Difference{
		Minuend:    &trade.in.clock,
		Subtrahend: &trade.in.epoch,
	}

	tradeRate := &calculus.Ratio{Numerator: tradeCount, Denominator: elapsed}
	grossNotionalRate := &calculus.Ratio{Numerator: grossNotional, Denominator: elapsed}
	netNotionalRate := &calculus.Ratio{Numerator: netNotional, Denominator: elapsed}
	buyNotionalRate := &calculus.Ratio{Numerator: buyNotional.Read(), Denominator: elapsed}
	sellNotionalRate := &calculus.Ratio{Numerator: sellNotional.Read(), Denominator: elapsed}

	midpointLogReturn := &nmtypes.Chain{
		A: &calculus.Ratio{Numerator: &in.midpoint, Denominator: &in.priorMid},
		B: calculus.Log{},
	}
	flowAlignedReturn := &nmtypes.Product{
		A: midpointLogReturn,
		B: &nmtypes.Chain{A: netNotional, B: calculus.Sign{}},
	}
	midpointResponse := &calculus.Ratio{
		Numerator:   midpointLogReturn,
		Denominator: netNotional,
	}

	estimator := &equation.CausalResidual{Key: keyFn}

	trade.number = nomagique.Number(&nmtypes.Chain{
		A: &nmtypes.Require{When: notional},

		B: &nmtypes.Split{
			A: &nmtypes.Split{
				A: &nmtypes.Labelled{
					Names: map[string]string{"total": "executed_quantity:buy"},
					Node:  buyQty,
				},
				B: &nmtypes.Labelled{
					Names: map[string]string{"total": "executed_quantity:sell"},
					Node:  sellQty,
				},
			},
			B: &nmtypes.Split{
				A: &nmtypes.Labelled{
					Names: map[string]string{"total": "trade_count:buy"},
					Node:  buyCount,
				},
				B: &nmtypes.Labelled{
					Names: map[string]string{"total": "trade_count:sell"},
					Node:  sellCount,
				},
			},
			C: &nmtypes.Split{
				A: &nmtypes.Labelled{
					Names: map[string]string{"total": "aggressive_notional:buy"},
					Node:  buyNotional,
				},
				B: &nmtypes.Labelled{
					Names: map[string]string{"total": "aggressive_notional:sell"},
					Node:  sellNotional,
				},
			},
		},

		C: &nmtypes.Split{
			A: &nmtypes.Split{
				A: &nmtypes.Report{
					Label: "trade_count", Unit: "count", Timescale: "instantaneous",
					Value: tradeCount,
				},
				B: &nmtypes.Report{
					Label: "gross_notional", Unit: "rate", Timescale: "instantaneous",
					Value: grossNotional,
				},
			},
			B: &nmtypes.Split{
				A: &nmtypes.Report{
					Label: "net_notional", Unit: "rate", Timescale: "instantaneous",
					Value: netNotional,
				},
				B: &nmtypes.Report{
					Label: "gross_executed_quantity", Unit: "count", Timescale: "instantaneous",
					Value: grossQty,
				},
			},
			C: &nmtypes.Report{
				Label: "net_executed_quantity", Unit: "count", Timescale: "instantaneous",
				Value: netQty,
			},
		},

		D: &nmtypes.Chain{
			A: &nmtypes.Split{
				A: &nmtypes.Report{
					Label: "cumulative_volume_delta", Unit: "count", Timescale: "instantaneous",
					Value: netQty,
				},
				B: &nmtypes.Report{
					Label: "cumulative_notional_delta", Unit: "rate", Timescale: "instantaneous",
					Value: netNotional,
				},
				C: &nmtypes.Report{
					Label: "signed_count_fraction", Unit: "dimensionless", Timescale: "instantaneous",
					Value: signedCountFraction,
				},
				D: &nmtypes.Report{
					Label: "mean_trade_notional", Unit: "rate", Timescale: "instantaneous",
					Value: meanTradeNotional,
				},
			},
			B: &nmtypes.Chain{
				A: &nmtypes.Report{
					Label: "signed_net_fraction", Unit: "dimensionless", Timescale: "instantaneous",
					Value: signedNetFraction,
				},
				B: &nmtypes.Labelled{
					Prefix: "signed_net_fraction_",
					Node:   estimator,
				},
			},
			C: &nmtypes.Split{
				A: &nmtypes.Split{
					A: &nmtypes.Report{
						Label: "gross_notional_rate", Unit: "per_second", Timescale: "per_second",
						Value: grossNotionalRate, Defined: elapsed,
					},
					B: &nmtypes.Report{
						Label: "net_notional_rate", Unit: "per_second", Timescale: "per_second",
						Value: netNotionalRate, Defined: elapsed,
					},
				},
				B: &nmtypes.Split{
					A: &nmtypes.Report{
						Label: "buy_notional_rate", Unit: "per_second", Timescale: "per_second",
						Value: buyNotionalRate, Defined: elapsed,
					},
					B: &nmtypes.Report{
						Label: "sell_notional_rate", Unit: "per_second", Timescale: "per_second",
						Value: sellNotionalRate, Defined: elapsed,
					},
					C: &nmtypes.Report{
						Label: "trade_rate", Unit: "per_second", Timescale: "per_second",
						Value: tradeRate, Defined: elapsed,
					},
					D: &nmtypes.Report{
						Label: "cvd_epoch_from", Unit: "second", Timescale: "instantaneous",
						Value: &in.epoch,
					},
				},
			},
			D: &nmtypes.Chain{
				A: &nmtypes.Split{
					A: &nmtypes.Report{
						Label: "midpoint_log_return", Unit: "dimensionless", Timescale: "instantaneous",
						Value: midpointLogReturn,
						Defined: &nmtypes.Product{A: &in.quoted, B: &in.priorMid},
					},
					B: &nmtypes.Split{
						A: &nmtypes.Report{
							Label: "flow_aligned_midpoint_return", Unit: "dimensionless", Timescale: "instantaneous",
							Value: flowAlignedReturn,
							Defined: &nmtypes.Product{A: &in.quoted, B: &in.priorMid},
						},
						B: &nmtypes.Report{
							Label: "midpoint_response_per_net_notional", Unit: "dimensionless", Timescale: "instantaneous",
							Value: midpointResponse,
							Defined: &nmtypes.Product{A: &in.quoted, B: &in.priorMid},
						},
					},
					C: &nmtypes.Labelled{
						Names: map[string]string{"velocity": "net_notional_rate_velocity"},
						Node: &temporal.Velocity{
							Source: netNotionalRate,
							Clock:  &in.clock,
							Key:    keyFn,
						},
					},
				},
				B: &data.Projection{
					Source:    "cvd",
					Identity:  trade.identity,
					Rejection: errUnmeasurable,
				},
			},
		},
	})

	return trade
}

/*
SetQuote installs the shared top-of-book quote source.
*/
func (trade *Trade) SetQuote(quote func(symbol string) (bid, ask *decimal.Decimal)) {
	trade.mutex.Lock()
	defer trade.mutex.Unlock()

	trade.quote = quote
}

/*
Step writes one trade onto the composition's slots, advances it once, and
returns the Measurement its terminal projection published.
*/
func (trade *Trade) Step(tradeData kraken.TradeData) *data.Measurement[float64] {
	trade.mutex.Lock()
	defer trade.mutex.Unlock()

	trade.symbol = tradeData.Symbol
	trade.at = tradeData.Timestamp

	price := tradeData.Price.Float64()
	qty := tradeData.Qty

	if price <= 0 || qty <= 0 {
		return &data.Measurement[float64]{Err: errUnmeasurable}
	}

	trade.in.price.Value = nmtypes.Number(price)
	trade.in.quantity.Value = nmtypes.Number(qty)
	trade.in.buy.Value = 0
	trade.in.sell.Value = 0

	if tradeData.Side == "sell" {
		trade.in.sell.Value = 1
	} else {
		trade.in.buy.Value = 1
	}

	trade.in.clock.Observe(tradeData.Timestamp)

	if trade.in.epoch.Seconds() == 0 {
		trade.in.epoch.Observe(tradeData.Timestamp)
	}

	trade.observeQuote(tradeData.Symbol)
	trade.number.Step(0)

	return trade.number.Measurement()
}

func (trade *Trade) identity() (string, string, time.Time, time.Time) {
	// The observation window start is the symbol epoch held as a precise
	// wall-clock time.Time. Reconstructing it from epoch.Seconds() —
	// float64(ns)/1e9 — and truncating back through int64 loses sub-microsecond
	// precision, so on a symbol's first measurement the reconstructed From could
	// land a fraction of a microsecond AFTER At and look like a window that
	// begins after its own event. Read the stored instant directly instead.
	from := trade.in.epoch.Time()

	// Defensive guard: the window must never begin after the event it measures.
	if from.After(trade.at) {
		from = trade.at
	}

	return trade.symbol + ":cvd:" + trade.at.Format(time.RFC3339Nano),
		trade.symbol,
		trade.at,
		from
}

func (trade *Trade) observeQuote(symbol string) {
	trade.in.quoted.Value = 0

	if trade.quote == nil {
		return
	}

	bid, ask := trade.quote(symbol)

	if bid == nil || ask == nil {
		return
	}

	midpoint := (bid.Float64() + ask.Float64()) / 2

	trade.in.midpoint.Value = nmtypes.Number(midpoint)
	trade.in.priorMid.Value = nmtypes.Number(trade.priorMd[symbol])
	trade.in.quoted.Value = 1

	trade.priorMd[symbol] = midpoint
}

/*
Close releases resources held by the Trade processor.
*/
func (trade *Trade) Close() error {
	return nil
}
