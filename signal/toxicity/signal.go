package toxicity

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Toxicity tracks whether near-touch liquidity is sincere, retreating, or bluffing
from level3 order events corroborated by the public trade tape.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	trades *Trade
	level3 *Level3
}

func NewSignal(ctx context.Context, api *websocket.API) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		trades: NewTrade(ctx, api),
		level3: NewLevel3(ctx, api),
	}
}

func (signal *Signal) Measure(
	thesis *types.Thesis,
) *types.Thesis {
	trades := signal.trades.cache
	books := signal.level3.cache
	out := datura.Map[datura.Map[*decimal.Decimal]]{}

	for _, trade := range trades {
		if trade.Price.Sign() <= 0 {
			continue
		}

		for _, book := range books {
			if book.Name == trade.Pair {
				if out[trade.Pair] == nil {
					out[trade.Pair] = datura.Map[*decimal.Decimal]{
						"touch":   decimal.NewFromFloat64(0),
						"bestBid": decimal.NewFromFloat64(0),
						"bestAsk": decimal.NewFromFloat64(0),
						"volume":  decimal.NewFromFloat64(0),
						"fillBid": decimal.NewFromFloat64(0),
						"fillAsk": decimal.NewFromFloat64(0),
					}
				}

				out = utils.Add(out, trade.Volume, trade.Pair, "volume")

				if trade.Price.Cmp(book.BestBid().Price) == 0 {
					out = utils.Add(out, book.BestBid().Price.Mul(trade.Volume), trade.Pair, "fillBid")
				}

				if trade.Price.Cmp(book.BestAsk().Price) == 0 {
					out = utils.Add(out, book.BestAsk().Price.Mul(trade.Volume), trade.Pair, "fillAsk")
				}
			}
		}
	}

	for _, book := range books {
		out[book.Name]["touch"] = book.BestBid().Quantity
		out[book.Name]["bestBid"] = book.BestBid().Price
		out[book.Name]["bestAsk"] = book.BestAsk().Price
		out[book.Name]["touchBid"] = book.BestBid().Quantity
		out[book.Name]["touchAsk"] = book.BestAsk().Quantity

		for _, side := range []string{"Bid", "Ask"} {
			if prev, ok := out[book.Name]["prev"+side]; ok && prev.Cmp(
				out[book.Name]["touch"+side],
			) > 0 {
				out[book.Name]["retreating"+side] = prev.Sub(
					out[book.Name]["touch"+side],
				)

				matched := false

				for _, trade := range trades {
					if trade.Pair == book.Name && trade.Price.Cmp(out[book.Name]["best"+side]) == 0 {
						matched = true
					}
				}

				if !matched {
					out = utils.Add(
						out, out[book.Name]["retreating"+side], book.Name, "cancel"+side,
					)
				}
			}
		}

		out[book.Name]["prevBid"] = book.BestBid().Quantity
		out[book.Name]["prevAsk"] = book.BestAsk().Quantity
	}

	signal.trades.cache = signal.trades.cache[:0]
	signal.level3.cache = signal.level3.cache[:0]

	thesis.Signals.Store("trades", trades)
	thesis.Signals.Store("books", books)
	thesis.Measurements.Store("toxicity", out)

	return thesis
}

func (signal *Signal) Close() (err error) {
	err = errnie.Error(errnie.Err(
		errnie.Internal,
		"signal: close failed",
		nil,
	))

	signal.cancel()
	return err
}
