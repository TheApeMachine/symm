package depthflow

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
)

/*
Reading is one displayed-depth observation of the executable touch.
*/
type Reading struct {
	BidNotional, AskNotional, Notional float64
	Imbalance                          float64
	HasImbalance                       bool
	AddedBid, AddedAsk                 float64
	RemovedBid, RemovedAsk             float64
	NetBid, NetAsk                     float64
	Turnover                           float64
	NetChangeRate                      float64
	SignedFlow                         float64
	HasFlow                            bool
	ImbalanceBase, ImbalanceZ          float64
	HasImbalanceBase                   bool
}

type path struct {
	hasPrev   bool
	prevBid   float64
	prevAsk   float64
	prevAt    int64
	imbalance core.Primitive
}

/*
Book measures displayed notional and its mutation between comparable touches.
*/
type Book struct {
	*core.PrimitiveError

	paths map[string]*path
	out   Reading
}

func NewBook() *Book {
	return &Book{
		PrimitiveError: core.NewPrimitiveError(),
		paths:          make(map[string]*path),
	}
}

func (book *Book) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			quote := *(*Quote)(arriving)

			if quote.Bid <= 0 || quote.Ask <= 0 {
				continue
			}

			state := book.paths[quote.Symbol]

			if state == nil {
				state = &path{imbalance: adaptive.NewBaseline(adaptive.NewWindow())}
				book.paths[quote.Symbol] = state
			}

			bidNotional := quote.Bid * quote.BidQty
			askNotional := quote.Ask * quote.AskQty
			total := bidNotional + askNotional
			reading := Reading{
				BidNotional: bidNotional,
				AskNotional: askNotional,
				Notional:    total,
			}

			if total > 0 {
				reading.Imbalance = (bidNotional - askNotional) / total
				reading.HasImbalance = true

				var baseline adaptive.BaselineReading

				for out := range state.imbalance.Next(sequence.NewOne(unsafe.Pointer(&reading.Imbalance)).Next(nil)) {
					baseline = *(*adaptive.BaselineReading)(out)
				}

				if err := state.imbalance.Error(); err != nil {
					book.Error(err)
					return
				}

				if baseline.HasPrior {
					reading.HasImbalanceBase = true
					reading.ImbalanceBase = baseline.Baseline
					reading.ImbalanceZ = baseline.ZScore
				}
			}

			if state.hasPrev && quote.At > state.prevAt {
				deltaBid := bidNotional - state.prevBid
				deltaAsk := askNotional - state.prevAsk
				reading.NetBid = deltaBid
				reading.NetAsk = deltaAsk
				reading.AddedBid = math.Max(deltaBid, 0)
				reading.AddedAsk = math.Max(deltaAsk, 0)
				reading.RemovedBid = math.Max(-deltaBid, 0)
				reading.RemovedAsk = math.Max(-deltaAsk, 0)
				reading.HasFlow = true

				dt := float64(quote.At-state.prevAt) / 1e9
				ref := (total + state.prevBid + state.prevAsk) / 2
				mutation := reading.AddedBid + reading.AddedAsk + reading.RemovedBid + reading.RemovedAsk

				if dt > 0 && ref > 0 {
					reading.Turnover = mutation / (ref * dt)
					reading.NetChangeRate = (total - state.prevBid - state.prevAsk) / (ref * dt)
					reading.SignedFlow = (deltaBid - deltaAsk) / (ref * dt)
				}
			}

			state.prevBid = bidNotional
			state.prevAsk = askNotional
			state.prevAt = quote.At
			state.hasPrev = true
			book.out = reading

			if !yield(unsafe.Pointer(&book.out)) {
				return
			}
		}
	}
}
