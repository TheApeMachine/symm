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
DispositionReading is one touch-to-touch liquidity disposition.
*/
type DispositionReading struct {
	Bid, Ask, BidQty, AskQty               float64
	PrevBid, PrevAsk                       float64
	HasPrev                                bool
	BidLogChange, AskLogChange             float64
	HasBidLog, HasAskLog                   bool
	RetreatedBidQty, RetreatedAskQty       float64
	WithdrawnBidQty, WithdrawnAskQty       float64
	ReplenishedBidQty, ReplenishedAskQty   float64
	RetreatBidFrac, RetreatAskFrac         float64
	WithdrawBidFrac, WithdrawAskFrac       float64
	RetreatBidRate, RetreatAskRate         float64
	WithdrawBidRate, WithdrawAskRate       float64
	HasRates                               bool
	WithdrawBidBase, WithdrawAskBase       float64
	WithdrawBidZ, WithdrawAskZ             float64
	HasWithdrawBidBase, HasWithdrawAskBase bool
}

type dispositionPath struct {
	hasPrev    bool
	prevBid    float64
	prevAsk    float64
	prevBidQty float64
	prevAskQty float64
	prevAt     int64
	bidBase    core.Primitive
	askBase    core.Primitive
}

/*
Disposition attributes previously displayed touch quantity.
*/
type Disposition struct {
	*core.PrimitiveError

	paths map[string]*dispositionPath
	out   DispositionReading
}

func NewDisposition() *Disposition {
	return &Disposition{
		PrimitiveError: core.NewPrimitiveError(),
		paths:          make(map[string]*dispositionPath),
	}
}

func (disposition *Disposition) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			quote := *(*Quote)(arriving)

			if quote.Bid <= 0 || quote.Ask <= 0 {
				continue
			}

			state := disposition.paths[quote.Symbol]

			if state == nil {
				state = &dispositionPath{
					bidBase: adaptive.NewBaseline(adaptive.NewWindow()),
					askBase: adaptive.NewBaseline(adaptive.NewWindow()),
				}
				disposition.paths[quote.Symbol] = state
			}

			reading := DispositionReading{
				Bid:    quote.Bid,
				Ask:    quote.Ask,
				BidQty: quote.BidQty,
				AskQty: quote.AskQty,
			}

			if state.hasPrev {
				if quote.At <= state.prevAt {
					continue
				}

				reading.HasPrev = true
				reading.PrevBid = state.prevBid
				reading.PrevAsk = state.prevAsk
				dt := float64(quote.At-state.prevAt) / 1e9

				if state.prevBid > 0 && quote.Bid > 0 {
					reading.BidLogChange = math.Log(quote.Bid / state.prevBid)
					reading.HasBidLog = true
				}

				if state.prevAsk > 0 && quote.Ask > 0 {
					reading.AskLogChange = math.Log(quote.Ask / state.prevAsk)
					reading.HasAskLog = true
				}

				if quote.Bid < state.prevBid {
					reading.RetreatedBidQty = state.prevBidQty
					reading.RetreatBidFrac = 1

					if dt > 0 {
						reading.RetreatBidRate = state.prevBidQty / dt
						reading.HasRates = true
					}
				}

				if quote.Bid == state.prevBid && quote.BidQty < state.prevBidQty {
					withdrawn := state.prevBidQty - quote.BidQty
					reading.WithdrawnBidQty = withdrawn

					if state.prevBidQty > 0 {
						reading.WithdrawBidFrac = withdrawn / state.prevBidQty
					}

					if dt > 0 {
						reading.WithdrawBidRate = withdrawn / dt
						reading.HasRates = true
					}
				}

				if quote.Bid == state.prevBid && quote.BidQty > state.prevBidQty {
					reading.ReplenishedBidQty = quote.BidQty - state.prevBidQty
				}

				if quote.Ask > state.prevAsk {
					reading.RetreatedAskQty = state.prevAskQty
					reading.RetreatAskFrac = 1

					if dt > 0 {
						reading.RetreatAskRate = state.prevAskQty / dt
						reading.HasRates = true
					}
				}

				if quote.Ask == state.prevAsk && quote.AskQty < state.prevAskQty {
					withdrawn := state.prevAskQty - quote.AskQty
					reading.WithdrawnAskQty = withdrawn

					if state.prevAskQty > 0 {
						reading.WithdrawAskFrac = withdrawn / state.prevAskQty
					}

					if dt > 0 {
						reading.WithdrawAskRate = withdrawn / dt
						reading.HasRates = true
					}
				}

				if quote.Ask == state.prevAsk && quote.AskQty > state.prevAskQty {
					reading.ReplenishedAskQty = quote.AskQty - state.prevAskQty
				}

				if reading.WithdrawBidFrac > 0 {
					var baseline adaptive.BaselineReading

					for out := range state.bidBase.Next(sequence.NewOne(unsafe.Pointer(&reading.WithdrawBidFrac)).Next(nil)) {
						baseline = *(*adaptive.BaselineReading)(out)
					}

					if err := state.bidBase.Error(); err != nil {
						disposition.Error(err)
						return
					}

					if baseline.HasPrior {
						reading.HasWithdrawBidBase = true
						reading.WithdrawBidBase = baseline.Baseline
						reading.WithdrawBidZ = baseline.ZScore
					}
				}

				if reading.WithdrawAskFrac > 0 {
					var baseline adaptive.BaselineReading

					for out := range state.askBase.Next(sequence.NewOne(unsafe.Pointer(&reading.WithdrawAskFrac)).Next(nil)) {
						baseline = *(*adaptive.BaselineReading)(out)
					}

					if err := state.askBase.Error(); err != nil {
						disposition.Error(err)
						return
					}

					if baseline.HasPrior {
						reading.HasWithdrawAskBase = true
						reading.WithdrawAskBase = baseline.Baseline
						reading.WithdrawAskZ = baseline.ZScore
					}
				}
			}

			state.prevBid = quote.Bid
			state.prevAsk = quote.Ask
			state.prevBidQty = quote.BidQty
			state.prevAskQty = quote.AskQty
			state.prevAt = quote.At
			state.hasPrev = true
			disposition.out = reading

			if !yield(unsafe.Pointer(&disposition.out)) {
				return
			}
		}
	}
}
