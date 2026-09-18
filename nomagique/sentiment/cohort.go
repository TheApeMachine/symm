package sentiment

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
Reading is one last-price observation plus the current cohort cut when the
arriving symbol belongs to the configured population.
*/
type Reading struct {
	Last                                             float64
	HasCohort                                        bool
	Valid, Advance, Decline, Unchanged, Excluded     float64
	Breadth, AdvanceFrac, DeclineFrac, UnchangedFrac float64
	Agreement, Consensus, Participation              float64
	HasAgreement                                     bool
	Median, MedianAbs, MeanAbs, RMS, MAD             float64
	LargestAbs, LargestSigned, LargestShare          float64
	SignedFrac                                       float64
	SignedBase, SignedDiv, SignedZ                   float64
	HasSignedBase                                    bool
}

type member struct {
	price  float64
	change float64
	has    bool
	ready  bool
}

/*
Cohort measures last price and, when a population is configured, the
cross-section of member log returns against each member's prior price.
*/
type Cohort struct {
	*core.PrimitiveError

	admit    map[string]struct{}
	members  map[string]*member
	baseline core.Primitive
	median   core.Primitive
	out      Reading
}

func NewCohort(names ...string) *Cohort {
	admit := make(map[string]struct{}, len(names))

	for _, name := range names {
		if name == "" {
			continue
		}

		admit[name] = struct{}{}
	}

	return &Cohort{
		PrimitiveError: core.NewPrimitiveError(),
		admit:          admit,
		members:        make(map[string]*member),
		baseline:       adaptive.NewBaseline(adaptive.NewWindow()),
		median:         statistic.NewMedian(),
	}
}

func (cohort *Cohort) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			tick := *(*Tick)(arriving)

			if tick.Last <= 0 {
				continue
			}

			reading := Reading{Last: tick.Last}

			if len(cohort.admit) == 0 {
				cohort.out = reading

				if !yield(unsafe.Pointer(&cohort.out)) {
					return
				}

				continue
			}

			if _, admitted := cohort.admit[tick.Symbol]; !admitted {
				continue
			}

			state := cohort.members[tick.Symbol]

			if state == nil {
				state = &member{}
				cohort.members[tick.Symbol] = state
			}

			if state.has && state.price > 0 {
				state.change = math.Log(tick.Last / state.price)
				state.ready = true
			}

			state.price = tick.Last
			state.has = true

			changes := make([]float64, 0, len(cohort.admit))
			advance := 0.0
			decline := 0.0
			unchanged := 0.0
			absSum := 0.0
			energy := 0.0
			largest := 0.0
			largestSigned := 0.0
			ties := 0.0

			for name := range cohort.admit {
				held := cohort.members[name]

				if held == nil || !held.ready {
					continue
				}

				change := held.change
				changes = append(changes, change)
				absChange := math.Abs(change)
				absSum += absChange
				energy += change * change

				if change > 0 {
					advance++
				}

				if change < 0 {
					decline++
				}

				if change == 0 {
					unchanged++
				}

				if absChange > largest {
					largest = absChange
					largestSigned = change
					ties = 1
				}

				if absChange == largest && change != largestSigned {
					ties++
				}
			}

			valid := float64(len(changes))
			reading.Excluded = float64(len(cohort.admit)) - valid

			if valid == 0 {
				cohort.out = reading

				if !yield(unsafe.Pointer(&cohort.out)) {
					return
				}

				continue
			}

			reading.HasCohort = true
			reading.Valid = valid
			reading.Advance = advance
			reading.Decline = decline
			reading.Unchanged = unchanged
			reading.AdvanceFrac = advance / valid
			reading.DeclineFrac = decline / valid
			reading.UnchangedFrac = unchanged / valid
			reading.Breadth = (advance - decline) / valid
			reading.SignedFrac = reading.Breadth
			reading.Participation = (advance + decline) / valid
			reading.MeanAbs = absSum / valid
			reading.RMS = math.Sqrt(energy / valid)
			reading.LargestAbs = largest

			if ties == 1 {
				reading.LargestSigned = largestSigned
			}

			if absSum > 0 {
				reading.LargestShare = largest / absSum
			}

			moved := advance + decline

			if moved > 0 {
				reading.HasAgreement = true
				reading.Consensus = math.Abs(advance-decline) / moved
				reading.Agreement = math.Max(advance, decline) / moved
			}

			var median float64

			for out := range cohort.median.Next(sequence.NewValues(changes...).Next(nil)) {
				median = *(*float64)(out)
			}

			if err := cohort.median.Error(); err != nil {
				cohort.Error(err)
				return
			}

			reading.Median = median
			absChanges := make([]float64, len(changes))
			deviations := make([]float64, len(changes))

			for index, change := range changes {
				absChanges[index] = math.Abs(change)
				deviations[index] = math.Abs(change - median)
			}

			for out := range cohort.median.Next(sequence.NewValues(absChanges...).Next(nil)) {
				reading.MedianAbs = *(*float64)(out)
			}

			if err := cohort.median.Error(); err != nil {
				cohort.Error(err)
				return
			}

			for out := range cohort.median.Next(sequence.NewValues(deviations...).Next(nil)) {
				reading.MAD = *(*float64)(out)
			}

			if err := cohort.median.Error(); err != nil {
				cohort.Error(err)
				return
			}

			var baseline adaptive.BaselineReading

			for out := range cohort.baseline.Next(sequence.NewOne(unsafe.Pointer(&reading.SignedFrac)).Next(nil)) {
				baseline = *(*adaptive.BaselineReading)(out)
			}

			if err := cohort.baseline.Error(); err != nil {
				cohort.Error(err)
				return
			}

			if baseline.HasPrior {
				reading.HasSignedBase = true
				reading.SignedBase = baseline.Baseline
				reading.SignedDiv = baseline.Residual
				reading.SignedZ = baseline.ZScore
			}

			cohort.out = reading

			if !yield(unsafe.Pointer(&cohort.out)) {
				return
			}
		}
	}
}
