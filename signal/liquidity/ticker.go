package liquidity

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

type tickerInput struct {
	bidPrice         calculus.Constant
	askPrice         calculus.Constant
	bidQty           calculus.Constant
	askQty           calculus.Constant
	bidNotional      calculus.Constant
	askNotional      calculus.Constant
	midpoint         calculus.Constant
	spread           calculus.Constant
	relativeSpread   calculus.Constant
	twoSidedNotional calculus.Constant
	imbalance        calculus.Constant
}

/*
Ticker is the touch-snapshot market entity. It maintains an online liquidity
model per symbol via a single nomagique.Number composition and projects data.Measurement outputs.
*/
type Ticker struct {
	mu     sync.Mutex
	number *nomagique.Pipeline
	div    *equation.MultivariateDivergence

	in     tickerInput
	symbol string
	at     time.Time
}

/*
NewTicker constructs the Ticker entity with a single inlined Number composition.
*/
func NewTicker() *Ticker {
	ticker := &Ticker{}

	keyFn := func() string { return ticker.symbol }
	ticker.div = equation.NewMultivariateDivergence(keyFn)
	in := &ticker.in

	ticker.number = nomagique.Number(&nmtypes.Chain{
		A: &nmtypes.Split{
			A: &nmtypes.Report{
				Label: "best_bid_price", Unit: "rate", Timescale: "instantaneous",
				Value: &in.bidPrice,
			},
			B: &nmtypes.Report{
				Label: "best_ask_price", Unit: "rate", Timescale: "instantaneous",
				Value: &in.askPrice,
			},
			C: &nmtypes.Report{
				Label: "touch_quantity:bid", Unit: "count", Timescale: "instantaneous",
				Value: &in.bidQty,
			},
			D: &nmtypes.Report{
				Label: "touch_quantity:ask", Unit: "count", Timescale: "instantaneous",
				Value: &in.askQty,
			},
		},
		B: &nmtypes.Split{
			A: &nmtypes.Report{
				Label: "touch_notional:bid", Unit: "rate", Timescale: "instantaneous",
				Value: &in.bidNotional,
			},
			B: &nmtypes.Report{
				Label: "touch_notional:ask", Unit: "rate", Timescale: "instantaneous",
				Value: &in.askNotional,
			},
			C: &nmtypes.Report{
				Label: "midpoint", Unit: "rate", Timescale: "instantaneous",
				Value: &in.midpoint,
			},
			D: &nmtypes.Report{
				Label: "spread", Unit: "rate", Timescale: "instantaneous",
				Value: &in.spread,
			},
		},
		C: &nmtypes.Split{
			A: &nmtypes.Report{
				Label: "relative_spread", Unit: "dimensionless", Timescale: "instantaneous",
				Value: &in.relativeSpread,
			},
			B: &nmtypes.Report{
				Label: "two_sided_touch_notional", Unit: "rate", Timescale: "instantaneous",
				Value: &in.twoSidedNotional,
			},
			C: &nmtypes.Report{
				Label: "touch_notional_imbalance", Unit: "dimensionless", Timescale: "instantaneous",
				Value: &in.imbalance,
			},
			D: ticker.div,
		},
		D: &data.Projection{
			Source:   "liquidity",
			Identity: ticker.identity,
		},
	})

	return ticker
}

/*
Step receives one market data point, executes the liquidity pipeline, and projects the measurement.
*/
func (ticker *Ticker) Step(trade kraken.TickerData) *data.Measurement[float64] {
	if trade.Bid == nil || trade.Ask == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("liquidity: ticker requires bid and ask")}
	}

	bidPrice := trade.Bid.Float64()
	askPrice := trade.Ask.Float64()
	bidQty := trade.BidQty
	askQty := trade.AskQty

	if bidPrice <= 0 || askPrice <= 0 || askPrice <= bidPrice {
		return &data.Measurement[float64]{Err: fmt.Errorf("liquidity: positive order violated (%f <= %f)", askPrice, bidPrice)}
	}

	ticker.mu.Lock()
	defer ticker.mu.Unlock()

	ticker.symbol = trade.Symbol
	ticker.at = trade.Timestamp

	bidNotional := bidPrice * bidQty
	askNotional := askPrice * askQty
	midpoint := (bidPrice + askPrice) / 2.0
	spread := askPrice - bidPrice
	relativeSpread := spread / midpoint
	twoSidedNotional := math.Min(bidNotional, askNotional)
	imbalance := (bidNotional - askNotional) / (bidNotional + askNotional)

	in := &ticker.in
	in.bidPrice.Value = nmtypes.Number(bidPrice)
	in.askPrice.Value = nmtypes.Number(askPrice)
	in.bidQty.Value = nmtypes.Number(bidQty)
	in.askQty.Value = nmtypes.Number(askQty)
	in.bidNotional.Value = nmtypes.Number(bidNotional)
	in.askNotional.Value = nmtypes.Number(askNotional)
	in.midpoint.Value = nmtypes.Number(midpoint)
	in.spread.Value = nmtypes.Number(spread)
	in.relativeSpread.Value = nmtypes.Number(relativeSpread)
	in.twoSidedNotional.Value = nmtypes.Number(twoSidedNotional)
	in.imbalance.Value = nmtypes.Number(imbalance)

	values := [3]float64{
		math.Log(bidNotional),
		math.Log(askNotional),
		math.Log(relativeSpread),
	}

	ticker.div.SetObservation(values, trade.Timestamp)
	ticker.number.Step(1.0)

	measurement := ticker.number.Measurement()

	if measurement != nil {
		if snr, hasSNR := ticker.div.Joint().SNR(); hasSNR {
			if measurement.Metadata == nil {
				measurement.Metadata = make(map[string]float64)
			}

			measurement.Metadata[data.MetadataMahalanobisSNR] = snr
		}
	}

	return measurement
}

func (ticker *Ticker) identity() (string, string, time.Time, time.Time) {
	return fmt.Sprintf("liquidity:%s:%d", ticker.symbol, ticker.at.UnixNano()),
		ticker.symbol,
		ticker.at,
		ticker.at
}

/*
Close releases resources held by the Ticker entity.
*/
func (ticker *Ticker) Close() error {
	return nil
}
