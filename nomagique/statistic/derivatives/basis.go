package derivatives

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
BasisReading holds basis, price difference, and open-interest metrics.
*/
type BasisReading struct {
	Basis           float64
	DerivativePrice float64
	ReferencePrice  float64
	OpenInterest    float64
	LogBasis        float64
	ReturnGap       float64
	OIChange        float64
	OIGrowth        float64
	BasisBaseline   float64
	BasisZScore     float64
}

/*
Basis computes relative basis between derivative last price and index reference price.
No structs, pure Value closure.
*/
type Basis types.Value[*Snapshot, BasisReading]

func NewBasis() Basis {
	var (
		prevLast  float64
		prevIndex float64
		prevOI    float64
		prevAt    int64
		hasPrev   bool
		sumBasis  float64
		count     float64
	)

	return func(snap *Snapshot) BasisReading {
		if snap == nil || snap.Index <= 0 {
			return BasisReading{}
		}

		currentBasis := (snap.Last - snap.Index) / snap.Index
		logBasis := 0.0
		if snap.Last > 0 && snap.Index > 0 {
			logBasis = math.Log(snap.Last / snap.Index)
		}

		sumBasis += currentBasis
		count++
		meanBasis := sumBasis / count
		zScore := 0.0
		if count > 1 {
			diff := currentBasis - meanBasis
			zScore = diff
		}

		oiChange := 0.0
		oiGrowth := 0.0
		returnGap := 0.0

		if hasPrev {
			oiChange = snap.OI - prevOI
			dt := float64(snap.At-prevAt) / 1e9
			if dt > 0 && prevOI > 0 && snap.OI > 0 {
				oiGrowth = math.Log(snap.OI/prevOI) / dt
			}
			if prevLast > 0 && snap.Last > 0 && prevIndex > 0 && snap.Index > 0 {
				derivReturn := math.Log(snap.Last / prevLast)
				refReturn := math.Log(snap.Index / prevIndex)
				returnGap = derivReturn - refReturn
			}
		}

		prevLast = snap.Last
		prevIndex = snap.Index
		prevOI = snap.OI
		prevAt = snap.At
		hasPrev = true

		return BasisReading{
			Basis:           currentBasis,
			DerivativePrice: snap.Last,
			ReferencePrice:  snap.Index,
			OpenInterest:    snap.OI,
			LogBasis:        logBasis,
			ReturnGap:       returnGap,
			OIChange:        oiChange,
			OIGrowth:        oiGrowth,
			BasisBaseline:   meanBasis,
			BasisZScore:     zScore,
		}
	}
}

type BasisValue types.Value[BasisReading, float64]

func NewBasisValue() BasisValue {
	return func(r BasisReading) float64 { return r.Basis }
}

type DerivativePrice types.Value[BasisReading, float64]

func NewDerivativePrice() DerivativePrice {
	return func(r BasisReading) float64 { return r.DerivativePrice }
}

type ReferencePrice types.Value[BasisReading, float64]

func NewReferencePrice() ReferencePrice {
	return func(r BasisReading) float64 { return r.ReferencePrice }
}

type OpenInterest types.Value[BasisReading, float64]

func NewOpenInterest() OpenInterest {
	return func(r BasisReading) float64 { return r.OpenInterest }
}

type LogBasis types.Value[BasisReading, float64]

func NewLogBasis() LogBasis {
	return func(r BasisReading) float64 { return r.LogBasis }
}

type ReturnGap types.Value[BasisReading, float64]

func NewReturnGap() ReturnGap {
	return func(r BasisReading) float64 { return r.ReturnGap }
}

type OIChange types.Value[BasisReading, float64]

func NewOIChange() OIChange {
	return func(r BasisReading) float64 { return r.OIChange }
}

type OIGrowth types.Value[BasisReading, float64]

func NewOIGrowth() OIGrowth {
	return func(r BasisReading) float64 { return r.OIGrowth }
}

type BasisBaseline types.Value[BasisReading, float64]

func NewBasisBaseline() BasisBaseline {
	return func(r BasisReading) float64 { return r.BasisBaseline }
}

type BasisZScore types.Value[BasisReading, float64]

func NewBasisZScore() BasisZScore {
	return func(r BasisReading) float64 { return r.BasisZScore }
}
