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

// errUnmeasurable is what a trade without a usable price and quantity carries:
// there is no execution to measure.
var errUnmeasurable = errors.New("cvd: trade requires a positive price and quantity")

/*
input is the set of slots one trade is written onto. Every quantity the
composition needs is read from these nodes, so an observation is data handed
to the pipeline rather than arithmetic performed before it.
*/
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

Keyed gives every symbol its own branch, so cumulative flow for one instrument
is never mixed with another's and the entity keeps no per-symbol map.
*/
func NewTrade() *Trade {
	trade := &Trade{priorMd: make(map[string]float64)}

	trade.number = nomagique.Number(&nmtypes.Keyed{
		Select: func() string { return trade.symbol },
		Build:  trade.branch,
	})

	return trade
}

/*
branch is one symbol's whole measurement, declared as one nested composition.

Reading it top to bottom: a trade's notional is gated onto the side it
executed, those gated flows accumulate, the accumulations are divided by the
elapsed clock into rates, the directional balance is standardized against its
own causal history, and the midpoint move is priced against the flow that
preceded it. Report names each quantity; Projection terminates the graph and
harvests every name.
*/
func (trade *Trade) branch() nmtypes.Node {
	in := &trade.in

	notional := &nmtypes.Product{A: &in.price, B: &in.quantity}

	return &nmtypes.Chain{
		// A trade with no price or no quantity is not a measurable execution.
		A: &nmtypes.Require{When: notional},

		// Accumulate executed flow, each side gated by the side the trade
		// actually executed on, and publish each accumulation under its name.
		B: &nmtypes.Split{
			A: &nmtypes.Split{
				A: &nmtypes.Labelled{
					Names: map[string]string{"total": "executed_quantity:buy"},
					Node: &calculus.Accumulator{
						Source: &calculus.Gate{Source: &in.quantity, When: &in.buy},
					},
				},
				B: &nmtypes.Labelled{
					Names: map[string]string{"total": "executed_quantity:sell"},
					Node: &calculus.Accumulator{
						Source: &calculus.Gate{Source: &in.quantity, When: &in.sell},
					},
				},
			},
			B: &nmtypes.Split{
				C: &nmtypes.Split{
					A: &nmtypes.Labelled{
						Names: map[string]string{"total": "trade_count:buy"},
						Node: &calculus.Accumulator{
							Source: &calculus.Gate{
								Source: &calculus.Constant{Value: 1}, When: &in.buy,
							},
						},
					},
					B: &nmtypes.Labelled{
						Names: map[string]string{"total": "trade_count:sell"},
						Node: &calculus.Accumulator{
							Source: &calculus.Gate{
								Source: &calculus.Constant{Value: 1}, When: &in.sell,
							},
						},
					},
					C: &nmtypes.Labelled{
						Names: map[string]string{"total": "trade_count"},
						Node:  &calculus.Accumulator{Source: &calculus.Constant{Value: 1}},
					},
				},
				A: &nmtypes.Labelled{
					Names: map[string]string{"total": "aggressive_notional:buy"},
					Node: &calculus.Accumulator{
						Source: &calculus.Gate{Source: notional, When: &in.buy},
					},
				},
				B: &nmtypes.Labelled{
					Names: map[string]string{"total": "aggressive_notional:sell"},
					Node: &calculus.Accumulator{
						Source: &calculus.Gate{Source: notional, When: &in.sell},
					},
				},
			},
		},

		// Derive the balances and rates from what was published above. Each
		// Ref reads a quantity by name, so nothing is recomputed and no local
		// variable carries a pointer between the two halves of the graph.
		C: &nmtypes.Split{
			A: &nmtypes.Split{
				A: &nmtypes.Report{
					Label: "gross_notional", Unit: "rate", Timescale: "instantaneous",
					Value: &nmtypes.Sum{
						A: &nmtypes.Ref{Name: "aggressive_notional:buy"},
						B: &nmtypes.Ref{Name: "aggressive_notional:sell"},
					},
				},
				B: &nmtypes.Report{
					Label: "net_notional", Unit: "rate", Timescale: "instantaneous",
					Value: &calculus.Difference{
						Minuend:    &nmtypes.Ref{Name: "aggressive_notional:buy"},
						Subtrahend: &nmtypes.Ref{Name: "aggressive_notional:sell"},
					},
				},
			},
			B: &nmtypes.Split{
				A: &nmtypes.Report{
					Label: "gross_executed_quantity", Unit: "count", Timescale: "instantaneous",
					Value: &nmtypes.Sum{
						A: &nmtypes.Ref{Name: "executed_quantity:buy"},
						B: &nmtypes.Ref{Name: "executed_quantity:sell"},
					},
				},
				B: &nmtypes.Report{
					Label: "net_executed_quantity", Unit: "count", Timescale: "instantaneous",
					Value: &calculus.Difference{
						Minuend:    &nmtypes.Ref{Name: "executed_quantity:buy"},
						Subtrahend: &nmtypes.Ref{Name: "executed_quantity:sell"},
					},
				},
			},
		},

		D: &nmtypes.Chain{
			// A Split is parallel, so a quantity derived from another must sit
			// in a LATER Chain stage than the one that published it.
			A: &nmtypes.Split{
				A: &nmtypes.Report{
					Label: "cumulative_volume_delta", Unit: "count", Timescale: "instantaneous",
					Value: &nmtypes.Ref{Name: "net_executed_quantity"},
				},
				B: &nmtypes.Report{
					Label: "cumulative_notional_delta", Unit: "rate", Timescale: "instantaneous",
					Value: &nmtypes.Ref{Name: "net_notional"},
				},
				C: &nmtypes.Report{
					Label: "signed_count_fraction", Unit: "dimensionless", Timescale: "instantaneous",
					Value: &calculus.Ratio{
						Numerator: &calculus.Difference{
							Minuend:    &nmtypes.Ref{Name: "trade_count:buy"},
							Subtrahend: &nmtypes.Ref{Name: "trade_count:sell"},
						},
						Denominator: &nmtypes.Ref{Name: "trade_count"},
					},
				},
				D: &nmtypes.Report{
					Label: "mean_trade_notional", Unit: "rate", Timescale: "instantaneous",
					Value: &calculus.Ratio{
						Numerator:   &nmtypes.Ref{Name: "gross_notional"},
						Denominator: &nmtypes.Ref{Name: "trade_count"},
					},
				},
			},
			// The share of executed notional that was directional, and its
			// standardization against its own causal history.
			B: &nmtypes.Chain{
				A: &nmtypes.Report{
					Label: "signed_net_fraction", Unit: "dimensionless", Timescale: "instantaneous",
					Value: &calculus.Ratio{
						Numerator:   &nmtypes.Ref{Name: "net_notional"},
						Denominator: &nmtypes.Ref{Name: "gross_notional"},
					},
				},
				B: &nmtypes.Labelled{
					Prefix: "signed_net_fraction_",
					Node:   &equation.CausalResidual{},
				},
			},

			// Rates, and how fast directional pressure is changing.
			C: &nmtypes.Split{
				A: &nmtypes.Split{
					A: &nmtypes.Report{
						Label: "gross_notional_rate", Unit: "per_second", Timescale: "per_second",
						Value: &calculus.Ratio{
							Numerator:   &nmtypes.Ref{Name: "gross_notional"},
							Denominator: trade.elapsed(),
						},
						Defined: trade.elapsed(),
					},
					B: &nmtypes.Report{
						Label: "net_notional_rate", Unit: "per_second", Timescale: "per_second",
						Value: &calculus.Ratio{
							Numerator:   &nmtypes.Ref{Name: "net_notional"},
							Denominator: trade.elapsed(),
						},
						Defined: trade.elapsed(),
					},
				},
				B: &nmtypes.Split{
					C: &nmtypes.Report{
						Label: "trade_rate", Unit: "per_second", Timescale: "per_second",
						Value: &calculus.Ratio{
							Numerator:   &nmtypes.Ref{Name: "trade_count"},
							Denominator: trade.elapsed(),
						},
						Defined: trade.elapsed(),
					},
					D: &nmtypes.Report{
						Label: "cvd_epoch_from", Unit: "second", Timescale: "instantaneous",
						Value: &in.epoch,
					},
					A: &nmtypes.Report{
						Label: "buy_notional_rate", Unit: "per_second", Timescale: "per_second",
						Value: &calculus.Ratio{
							Numerator:   &nmtypes.Ref{Name: "aggressive_notional:buy"},
							Denominator: trade.elapsed(),
						},
						Defined: trade.elapsed(),
					},
					B: &nmtypes.Report{
						Label: "sell_notional_rate", Unit: "per_second", Timescale: "per_second",
						Value: &calculus.Ratio{
							Numerator:   &nmtypes.Ref{Name: "aggressive_notional:sell"},
							Denominator: trade.elapsed(),
						},
						Defined: trade.elapsed(),
					},
				},
			},

			// The midpoint move, signed by the flow that preceded it and
			// priced per unit of that flow.
			D: &nmtypes.Split{
				A: &nmtypes.Report{
					Label: "midpoint_log_return", Unit: "dimensionless", Timescale: "instantaneous",
					Value: &nmtypes.Chain{
						A: &calculus.Ratio{Numerator: &in.midpoint, Denominator: &in.priorMid},
						B: calculus.Log{},
					},
					Defined: &nmtypes.Product{A: &in.quoted, B: &in.priorMid},
				},
				B: &nmtypes.Split{
					A: &nmtypes.Report{
						Label: "flow_aligned_midpoint_return", Unit: "dimensionless", Timescale: "instantaneous",
						Value: &nmtypes.Product{
							A: &nmtypes.Ref{Name: "midpoint_log_return"},
							B: &nmtypes.Chain{
								A: &nmtypes.Ref{Name: "net_notional"},
								B: calculus.Sign{},
							},
						},
						Defined: &nmtypes.Product{A: &in.quoted, B: &in.priorMid},
					},
					B: &nmtypes.Report{
						Label: "midpoint_response_per_net_notional", Unit: "dimensionless", Timescale: "instantaneous",
						Value: &calculus.Ratio{
							Numerator:   &nmtypes.Ref{Name: "midpoint_log_return"},
							Denominator: &nmtypes.Ref{Name: "net_notional"},
						},
						Defined: &nmtypes.Product{A: &in.quoted, B: &in.priorMid},
					},
				},
				C: &nmtypes.Labelled{
					Names: map[string]string{"velocity": "net_notional_rate_velocity"},
					Node: &temporal.Velocity{
						Source: &nmtypes.Ref{Name: "net_notional_rate"},
						Clock:  &in.clock,
					},
				},
			},

			D: &data.Projection{
				Source:    "cvd",
				Identity:  trade.identity,
				Rejection: errUnmeasurable,
			},
		},
	}
}

/* elapsed is the event time spanned since this symbol was first observed. */
func (trade *Trade) elapsed() nmtypes.Node {
	return &calculus.Difference{
		Minuend:    &trade.in.clock,
		Subtrahend: &trade.in.epoch,
	}
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

	trade.in.price.Value = nmtypes.Number(tradeData.Price.Float64())
	trade.in.quantity.Value = nmtypes.Number(tradeData.Qty)
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

/*
identity names the observation being published. The epoch slot carries the
first instant this symbol was seen, so From is read from the composition.
*/
func (trade *Trade) identity() (string, string, time.Time, time.Time) {
	return trade.symbol + ":cvd:" + trade.at.Format(time.RFC3339Nano),
		trade.symbol,
		trade.at,
		time.Unix(0, int64(trade.in.epoch.Seconds()*temporal.NanosPerSecond))
}

/*
observeQuote writes the contemporaneous and previous midpoint onto their slots.
quoted marks whether the pair is usable, which gates the quote-derived
readings.
*/
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
