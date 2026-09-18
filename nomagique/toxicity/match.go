package toxicity

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
)

/*
MatchReading is fill attribution against the retained touch.
*/
type MatchReading struct {
	Qty                            float64
	BracketQty                     float64
	MatchedBidQty, MatchedAskQty   float64
	FillBidQty, FillAskQty         float64
	FillBidFrac, FillAskFrac       float64
	FillBidRate, FillAskRate       float64
	HasRate                        bool
	FillBidBase, FillAskBase       float64
	FillBidZ, FillAskZ             float64
	HasFillBidBase, HasFillAskBase bool
}

type matchPath struct {
	quote      Quote
	hasQuote   bool
	bracket    float64
	matchedBid float64
	matchedAsk float64
	fillBid    float64
	fillAsk    float64
	prevAt     int64
	hasPrev    bool
	bidBase    core.Primitive
	askBase    core.Primitive
}

/*
Match attributes aggressive trades to the last observed touch.
*/
type Match struct {
	*core.PrimitiveError

	paths map[string]*matchPath
	out   MatchReading
}

func NewMatch() *Match {
	return &Match{
		PrimitiveError: core.NewPrimitiveError(),
		paths:          make(map[string]*matchPath),
	}
}

func (match *Match) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			arrival := *(*mixed)(arriving)
			symbol := arrival.Fill.Symbol

			if symbol == "" {
				symbol = arrival.Quote.Symbol
			}

			state := match.paths[symbol]

			if state == nil {
				state = &matchPath{
					bidBase: adaptive.NewBaseline(adaptive.NewWindow()),
					askBase: adaptive.NewBaseline(adaptive.NewWindow()),
				}
				match.paths[symbol] = state
			}

			if arrival.HasQuote {
				state.quote = arrival.Quote
				state.hasQuote = true
			}

			if !arrival.HasFill {
				continue
			}

			fill := arrival.Fill
			reading := MatchReading{Qty: fill.Qty}

			if state.hasQuote && fill.Price >= state.quote.Bid && fill.Price <= state.quote.Ask {
				state.bracket += fill.Qty
			}

			reading.BracketQty = state.bracket

			if state.hasQuote && fill.Side == "sell" && fill.Price == state.quote.Bid {
				state.matchedBid += fill.Qty
				capped := math.Min(state.matchedBid, state.quote.BidQty)
				state.fillBid = capped
				reading.FillBidFrac = 0

				if state.quote.BidQty > 0 {
					reading.FillBidFrac = capped / state.quote.BidQty
				}
			}

			if state.hasQuote && fill.Side == "buy" && fill.Price == state.quote.Ask {
				state.matchedAsk += fill.Qty
				capped := math.Min(state.matchedAsk, state.quote.AskQty)
				state.fillAsk = capped
				reading.FillAskFrac = 0

				if state.quote.AskQty > 0 {
					reading.FillAskFrac = capped / state.quote.AskQty
				}
			}

			reading.MatchedBidQty = state.matchedBid
			reading.MatchedAskQty = state.matchedAsk
			reading.FillBidQty = state.fillBid
			reading.FillAskQty = state.fillAsk

			if state.hasPrev && fill.At > state.prevAt {
				dt := float64(fill.At-state.prevAt) / 1e9

				if dt > 0 {
					reading.FillBidRate = state.fillBid / dt
					reading.FillAskRate = state.fillAsk / dt
					reading.HasRate = true
				}
			}

			if reading.FillBidFrac > 0 {
				var baseline adaptive.BaselineReading

				for out := range state.bidBase.Next(sequence.NewOne(unsafe.Pointer(&reading.FillBidFrac)).Next(nil)) {
					baseline = *(*adaptive.BaselineReading)(out)
				}

				if err := state.bidBase.Error(); err != nil {
					match.Error(err)
					return
				}

				if baseline.HasPrior {
					reading.HasFillBidBase = true
					reading.FillBidBase = baseline.Baseline
					reading.FillBidZ = baseline.ZScore
				}
			}

			if reading.FillAskFrac > 0 {
				var baseline adaptive.BaselineReading

				for out := range state.askBase.Next(sequence.NewOne(unsafe.Pointer(&reading.FillAskFrac)).Next(nil)) {
					baseline = *(*adaptive.BaselineReading)(out)
				}

				if err := state.askBase.Error(); err != nil {
					match.Error(err)
					return
				}

				if baseline.HasPrior {
					reading.HasFillAskBase = true
					reading.FillAskBase = baseline.Baseline
					reading.FillAskZ = baseline.ZScore
				}
			}

			state.prevAt = fill.At
			state.hasPrev = true
			match.out = reading

			if !yield(unsafe.Pointer(&match.out)) {
				return
			}
		}
	}
}
