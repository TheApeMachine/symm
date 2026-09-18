package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
LastPrice yields the arriving observation's price.
*/
type LastPrice struct {
	*core.PrimitiveError

	out float64
}

func NewLastPrice() *LastPrice {
	return &LastPrice{PrimitiveError: core.NewPrimitiveError()}
}

func (lastPrice *LastPrice) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			observation := *(*PriceObservation)(arriving)

			if observation.Value <= 0 {
				continue
			}

			lastPrice.out = observation.Value

			if !yield(unsafe.Pointer(&lastPrice.out)) {
				return
			}
		}
	}
}

/*
ObservationCount yields the retained path length of a pair reading.
*/
type ObservationCount struct {
	*core.PrimitiveError

	symbol string
	out    float64
}

func NewObservationCount(symbol ...string) *ObservationCount {
	count := &ObservationCount{PrimitiveError: core.NewPrimitiveError()}

	if len(symbol) > 0 {
		count.symbol = symbol[0]
	}

	return count
}

func (observationCount *ObservationCount) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*PairsReading)(arriving)

			if observationCount.symbol != "" && reading.Observation.Symbol != observationCount.symbol {
				continue
			}

			observationCount.out = reading.Path.Count

			if !yield(unsafe.Pointer(&observationCount.out)) {
				return
			}
		}
	}
}

/*
Signed yields the selected pair's signed correlation.
*/
type Signed struct {
	*core.PrimitiveError

	out float64
}

func NewSigned() *Signed {
	return &Signed{PrimitiveError: core.NewPrimitiveError()}
}

func (signed *Signed) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*PairsReading)(arriving)

			if !reading.Selected.Defined {
				continue
			}

			signed.out = reading.Selected.Correlation

			if !yield(unsafe.Pointer(&signed.out)) {
				return
			}
		}
	}
}

/*
AbsoluteCorrelation yields the selected pair's absolute correlation.
*/
type AbsoluteCorrelation struct {
	*core.PrimitiveError

	out float64
}

func NewAbsoluteCorrelation() *AbsoluteCorrelation {
	return &AbsoluteCorrelation{PrimitiveError: core.NewPrimitiveError()}
}

func (absoluteCorrelation *AbsoluteCorrelation) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*PairsReading)(arriving)

			if !reading.Selected.Defined {
				continue
			}

			absoluteCorrelation.out = math.Abs(reading.Selected.Correlation)

			if !yield(unsafe.Pointer(&absoluteCorrelation.out)) {
				return
			}
		}
	}
}

/*
PairCovariance yields the selected pair's unnormalized covariance.
*/
type PairCovariance struct {
	*core.PrimitiveError

	out float64
}

func NewPairCovariance() *PairCovariance {
	return &PairCovariance{PrimitiveError: core.NewPrimitiveError()}
}

func (pairCovariance *PairCovariance) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*PairsReading)(arriving)

			if !reading.Selected.Defined {
				continue
			}

			pairCovariance.out = reading.Selected.Covariance

			if !yield(unsafe.Pointer(&pairCovariance.out)) {
				return
			}
		}
	}
}

/*
ReferenceEnergy yields the selected pair's reference-path overlap energy.
*/
type ReferenceEnergy struct {
	*core.PrimitiveError

	out float64
}

func NewReferenceEnergy() *ReferenceEnergy {
	return &ReferenceEnergy{PrimitiveError: core.NewPrimitiveError()}
}

func (referenceEnergy *ReferenceEnergy) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*PairsReading)(arriving)

			if !reading.Selected.Defined {
				continue
			}

			referenceEnergy.out = reading.Selected.RightEnergy

			if !yield(unsafe.Pointer(&referenceEnergy.out)) {
				return
			}
		}
	}
}

/*
MeasuredEnergy yields the selected pair's measured-path overlap energy.
*/
type MeasuredEnergy struct {
	*core.PrimitiveError

	out float64
}

func NewMeasuredEnergy() *MeasuredEnergy {
	return &MeasuredEnergy{PrimitiveError: core.NewPrimitiveError()}
}

func (measuredEnergy *MeasuredEnergy) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*PairsReading)(arriving)

			if !reading.Selected.Defined {
				continue
			}

			measuredEnergy.out = reading.Selected.LeftEnergy

			if !yield(unsafe.Pointer(&measuredEnergy.out)) {
				return
			}
		}
	}
}

/*
ReferenceEnergyRate yields the selected pair's reference-path energy rate.
*/
type ReferenceEnergyRate struct {
	*core.PrimitiveError

	out float64
}

func NewReferenceEnergyRate() *ReferenceEnergyRate {
	return &ReferenceEnergyRate{PrimitiveError: core.NewPrimitiveError()}
}

func (referenceEnergyRate *ReferenceEnergyRate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*PairsReading)(arriving)

			if !reading.Selected.Defined {
				continue
			}

			if reading.Selected.RightEnergyRate <= 0 {
				continue
			}

			referenceEnergyRate.out = reading.Selected.RightEnergyRate

			if !yield(unsafe.Pointer(&referenceEnergyRate.out)) {
				return
			}
		}
	}
}

/*
MeasuredEnergyRate yields the selected pair's measured-path energy rate.
*/
type MeasuredEnergyRate struct {
	*core.PrimitiveError

	out float64
}

func NewMeasuredEnergyRate() *MeasuredEnergyRate {
	return &MeasuredEnergyRate{PrimitiveError: core.NewPrimitiveError()}
}

func (measuredEnergyRate *MeasuredEnergyRate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*PairsReading)(arriving)

			if !reading.Selected.Defined {
				continue
			}

			if reading.Selected.LeftEnergyRate <= 0 {
				continue
			}

			measuredEnergyRate.out = reading.Selected.LeftEnergyRate

			if !yield(unsafe.Pointer(&measuredEnergyRate.out)) {
				return
			}
		}
	}
}

/*
OverlapDensity yields the selected pair's overlap density.
*/
type OverlapDensity struct {
	*core.PrimitiveError

	out float64
}

func NewOverlapDensity() *OverlapDensity {
	return &OverlapDensity{PrimitiveError: core.NewPrimitiveError()}
}

func (overlapDensity *OverlapDensity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*PairsReading)(arriving)

			if !reading.Selected.Defined {
				continue
			}

			overlapDensity.out = reading.Selected.OverlapDensity

			if !yield(unsafe.Pointer(&overlapDensity.out)) {
				return
			}
		}
	}
}

/*
MeasuredReturns yields the selected pair's measured-path supported return count.
*/
type MeasuredReturns struct {
	*core.PrimitiveError

	out float64
}

func NewMeasuredReturns() *MeasuredReturns {
	return &MeasuredReturns{PrimitiveError: core.NewPrimitiveError()}
}

func (measuredReturns *MeasuredReturns) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*PairsReading)(arriving)

			if !reading.Selected.Defined {
				continue
			}

			measuredReturns.out = reading.Selected.LeftReturns

			if !yield(unsafe.Pointer(&measuredReturns.out)) {
				return
			}
		}
	}
}

/*
ReferenceReturns yields the selected pair's reference-path supported return count.
*/
type ReferenceReturns struct {
	*core.PrimitiveError

	out float64
}

func NewReferenceReturns() *ReferenceReturns {
	return &ReferenceReturns{PrimitiveError: core.NewPrimitiveError()}
}

func (referenceReturns *ReferenceReturns) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*PairsReading)(arriving)

			if !reading.Selected.Defined {
				continue
			}

			referenceReturns.out = reading.Selected.RightReturns

			if !yield(unsafe.Pointer(&referenceReturns.out)) {
				return
			}
		}
	}
}

/*
OverlapCount yields the selected pair's overlapping return-interval count.
*/
type OverlapCount struct {
	*core.PrimitiveError

	out float64
}

func NewOverlapCount() *OverlapCount {
	return &OverlapCount{PrimitiveError: core.NewPrimitiveError()}
}

func (overlapCount *OverlapCount) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*PairsReading)(arriving)

			if !reading.Selected.Defined {
				continue
			}

			overlapCount.out = reading.Selected.Support

			if !yield(unsafe.Pointer(&overlapCount.out)) {
				return
			}
		}
	}
}

/*
SharedTime yields the selected pair's shared observation span in seconds.
*/
type SharedTime struct {
	*core.PrimitiveError

	out float64
}

func NewSharedTime() *SharedTime {
	return &SharedTime{PrimitiveError: core.NewPrimitiveError()}
}

func (sharedTime *SharedTime) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*PairsReading)(arriving)

			if !reading.Selected.Defined {
				continue
			}

			sharedTime.out = reading.Selected.SharedTime

			if !yield(unsafe.Pointer(&sharedTime.out)) {
				return
			}
		}
	}
}

/*
PValue yields the selected pair's Fisher p-value.
*/
type PValue struct {
	*core.PrimitiveError

	out float64
}

func NewPValue() *PValue {
	return &PValue{PrimitiveError: core.NewPrimitiveError()}
}

func (pValue *PValue) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*PairsReading)(arriving)

			if !reading.Fisher.Defined {
				continue
			}

			pValue.out = reading.Fisher.PValue

			if !yield(unsafe.Pointer(&pValue.out)) {
				return
			}
		}
	}
}

/*
StandardError yields the selected pair's Fisher standard error.
*/
type StandardError struct {
	*core.PrimitiveError

	out float64
}

func NewStandardError() *StandardError {
	return &StandardError{PrimitiveError: core.NewPrimitiveError()}
}

func (standardError *StandardError) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*PairsReading)(arriving)

			if !reading.Fisher.Defined {
				continue
			}

			standardError.out = reading.Fisher.StandardError

			if !yield(unsafe.Pointer(&standardError.out)) {
				return
			}
		}
	}
}

/*
EnergyPair yields the measured and reference energy rates as a dividend/divisor
pair when both rates are positive.
*/
type EnergyPair struct {
	*core.PrimitiveError

	out [2]float64
}

func NewEnergyPair() *EnergyPair {
	return &EnergyPair{PrimitiveError: core.NewPrimitiveError()}
}

func (energyPair *EnergyPair) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*PairsReading)(arriving)

			if !reading.Selected.Defined {
				continue
			}

			if reading.Selected.LeftEnergyRate <= 0 {
				continue
			}

			if reading.Selected.RightEnergyRate <= 0 {
				continue
			}

			energyPair.out = [2]float64{
				reading.Selected.LeftEnergyRate,
				reading.Selected.RightEnergyRate,
			}

			if !yield(unsafe.Pointer(&energyPair.out)) {
				return
			}
		}
	}
}

/*
Admitted presents each admitted peer from a pair reading.
*/
type Admitted struct {
	*core.PrimitiveError

	out Peer
}

func NewAdmitted() *Admitted {
	return &Admitted{PrimitiveError: core.NewPrimitiveError()}
}

func (admitted *Admitted) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*PairsReading)(arriving)

			for _, peer := range reading.Peers {
				admitted.out = peer

				if !yield(unsafe.Pointer(&admitted.out)) {
					return
				}
			}
		}
	}
}

/*
FisherPoint maps a defined unsaturated correlation onto its Fisher-space
observation at the arrival's timestamp.
*/
type FisherPoint struct {
	*core.PrimitiveError

	out temporal.Observation
}

func NewFisherPoint() *FisherPoint {
	return &FisherPoint{PrimitiveError: core.NewPrimitiveError()}
}

func (fisherPoint *FisherPoint) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*PairsReading)(arriving)

			if !reading.Selected.Defined {
				continue
			}

			rho := reading.Selected.Correlation

			if rho <= -core.Unit || rho >= core.Unit {
				continue
			}

			fisherPoint.out = temporal.Observation{
				Value: math.Atanh(rho),
				At:    reading.Observation.At,
			}

			if !yield(unsafe.Pointer(&fisherPoint.out)) {
				return
			}
		}
	}
}

/*
EnergyPoint maps a defined positive energy-rate ratio onto its log-ratio
observation at the arrival's timestamp.
*/
type EnergyPoint struct {
	*core.PrimitiveError

	out temporal.Observation
}

func NewEnergyPoint() *EnergyPoint {
	return &EnergyPoint{PrimitiveError: core.NewPrimitiveError()}
}

func (energyPoint *EnergyPoint) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*PairsReading)(arriving)

			if !reading.Selected.Defined {
				continue
			}

			if reading.Selected.LeftEnergyRate <= 0 {
				continue
			}

			if reading.Selected.RightEnergyRate <= 0 {
				continue
			}

			energyPoint.out = temporal.Observation{
				Value: math.Log(reading.Selected.LeftEnergyRate / reading.Selected.RightEnergyRate),
				At:    reading.Observation.At,
			}

			if !yield(unsafe.Pointer(&energyPoint.out)) {
				return
			}
		}
	}
}
