package derivatives

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
)

/*
BasisReading is one derivative/reference geometry observation.
*/
type BasisReading struct {
	Last, Index, Mark, OI              float64
	Basis, LogBasis                    float64
	HasLog                             bool
	DILog, ISLog, DSLog, Closure       float64
	HasThree                           bool
	BasisBase, BasisZ                  float64
	HasBasisBase                       bool
	OIChange, OILogChange, OIGrowth    float64
	HasOIChange, HasOILog, HasOIGrowth bool
	OIGrowthBase                       float64
	HasOIGrowthBase                    bool
	BasisChange, BasisRate             float64
	HasBasisRate                       bool
	DerivReturn, RefReturn, ReturnGap  float64
	HasReturns                         bool
}

type basisPath struct {
	hasPrev   bool
	prevAt    int64
	prevLast  float64
	prevIndex float64
	prevOI    float64
	prevBasis float64
	basis     core.Primitive
	growth    core.Primitive
}

/*
Basis measures derivative/reference geometry and open-interest change.
*/
type Basis struct {
	*core.PrimitiveError

	paths map[string]*basisPath
	out   BasisReading
}

func NewBasis() *Basis {
	return &Basis{
		PrimitiveError: core.NewPrimitiveError(),
		paths:          make(map[string]*basisPath),
	}
}

func (basis *Basis) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			snapshot := *(*Snapshot)(arriving)

			if snapshot.Index <= 0 || snapshot.Last < 0 || snapshot.OI < 0 {
				continue
			}

			state := basis.paths[snapshot.Symbol]

			if state == nil {
				state = &basisPath{
					basis:  adaptive.NewBaseline(adaptive.NewWindow()),
					growth: adaptive.NewBaseline(adaptive.NewWindow()),
				}
				basis.paths[snapshot.Symbol] = state
			}

			if state.hasPrev && snapshot.At < state.prevAt {
				continue
			}

			current := (snapshot.Last - snapshot.Index) / snapshot.Index
			reading := BasisReading{
				Last:  snapshot.Last,
				Index: snapshot.Index,
				Mark:  snapshot.Mark,
				OI:    snapshot.OI,
				Basis: current,
			}

			if snapshot.Last > 0 {
				reading.LogBasis = math.Log(snapshot.Last / snapshot.Index)
				reading.HasLog = true
			}

			if snapshot.Last > 0 && snapshot.Mark > 0 {
				reading.DILog = math.Log(snapshot.Last / snapshot.Index)
				reading.ISLog = math.Log(snapshot.Index / snapshot.Mark)
				reading.DSLog = math.Log(snapshot.Last / snapshot.Mark)
				reading.Closure = reading.DSLog - reading.DILog - reading.ISLog
				reading.HasThree = true
			}

			var baseline adaptive.BaselineReading

			for out := range state.basis.Next(sequence.NewOne(unsafe.Pointer(&current)).Next(nil)) {
				baseline = *(*adaptive.BaselineReading)(out)
			}

			if err := state.basis.Error(); err != nil {
				basis.Error(err)
				return
			}

			if baseline.HasPrior {
				reading.HasBasisBase = true
				reading.BasisBase = baseline.Baseline
				reading.BasisZ = baseline.ZScore
			}

			advanced := state.hasPrev && snapshot.At > state.prevAt
			dt := 0.0

			if advanced {
				dt = float64(snapshot.At-state.prevAt) / 1e9
				reading.OIChange = snapshot.OI - state.prevOI
				reading.HasOIChange = true
			}

			if advanced && state.prevOI > 0 && snapshot.OI > 0 {
				reading.OILogChange = math.Log(snapshot.OI / state.prevOI)
				reading.HasOILog = true
			}

			if advanced && reading.HasOILog && dt > 0 {
				reading.OIGrowth = reading.OILogChange / dt
				reading.HasOIGrowth = true

				var growth adaptive.BaselineReading

				for out := range state.growth.Next(sequence.NewOne(unsafe.Pointer(&reading.OIGrowth)).Next(nil)) {
					growth = *(*adaptive.BaselineReading)(out)
				}

				if err := state.growth.Error(); err != nil {
					basis.Error(err)
					return
				}

				if growth.HasPrior {
					reading.HasOIGrowthBase = true
					reading.OIGrowthBase = growth.Baseline
				}
			}

			if advanced && dt > 0 {
				reading.BasisChange = current - state.prevBasis
				reading.BasisRate = reading.BasisChange / dt
				reading.HasBasisRate = true
			}

			if advanced && state.prevLast > 0 && snapshot.Last > 0 && state.prevIndex > 0 {
				reading.DerivReturn = math.Log(snapshot.Last / state.prevLast)
				reading.RefReturn = math.Log(snapshot.Index / state.prevIndex)
				reading.ReturnGap = reading.DerivReturn - reading.RefReturn
				reading.HasReturns = true
			}

			state.prevAt = snapshot.At
			state.prevLast = snapshot.Last
			state.prevIndex = snapshot.Index
			state.prevOI = snapshot.OI
			state.prevBasis = current
			state.hasPrev = true
			basis.out = reading

			if !yield(unsafe.Pointer(&basis.out)) {
				return
			}
		}
	}
}
