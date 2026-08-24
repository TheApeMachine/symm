package data

import (
	"math"
	"sort"
	"sync"
	"time"
)

const crossSectionPathLength = 32

/*
Member holds one key's retained observation facts: its latest and previous
relative changes, its observation timestamps, and its observation count.
*/
type memberState struct {
	latest       float64
	previous     float64
	at           time.Time
	from         time.Time
	observations int
}

/*
AggregateView is one cross-sectional aggregate with its causal estimator
state. Baseline, Divergence, ZScore, and Velocity describe the aggregate's own
history strictly before the current observation, so a current value can never
reduce its own apparent divergence.
*/
type AggregateView struct {
	Value      float64
	Baseline   float64
	Divergence float64
	ZScore     float64
	Velocity   float64
	Ready      bool
}

type aggregateEstimator struct {
	baseline  float64
	energy    float64
	lastValue float64
	lastAt    float64
	observations int
	hasValue  bool
}

/*
Snapshot is one immutable cross-sectional aggregation emitted after a value is
folded into a CrossSection. It carries only generic aggregate statistics:
counts, signed fractions, robust location and dispersion, extremes, ages, and
per-aggregate causal estimator views. Interpretation (breadth, leadership,
returns) belongs to the consumer, never to this container.
*/
type Snapshot struct {
	At   time.Time
	Count int
	// TotalMembers counts every key that has ever contributed an observation.
	TotalMembers int
	// Signed distribution of the latest member changes.
	PositiveCount int
	NegativeCount int
	ZeroCount     int
	// Location and dispersion of the latest member changes.
	SignedMedian   float64
	MeanAbsolute   float64
	MedianAbsolute float64
	Mad            float64
	MagnitudeMad   float64
	Iqr            float64
	Rms            float64
	// Extremes by magnitude.
	ExtremeKey           string
	ExtremeMagnitude     float64
	ExtremeSigned        float64
	ExtremeSecond        float64
	ExtremeTieCount      int
	SumMagnitude         float64
	// Ages in seconds since the observation time.
	MaxAge       float64
	MeanAge      float64
	MedianAge    float64
	MedianFromAge float64
	// Focal-relative comparisons, populated for the focal key when one is set.
	SameDirectionCount     int
	OppositeDirectionCount int
	ZeroDirectionCount     int
	PeerMedianAbsolute     float64
	PeerMad                float64
	FocalAge               float64
	FocalFromAge           float64
	// Causal estimator views over each aggregate's own history.
	Aggregates map[string]AggregateView
}

/*
CrossSection retains one bounded window of recent changes per key plus causal
estimator state per aggregate, and emits a generic Snapshot after each fold.
The container knows nothing about what the keys or values mean; it only keeps
per-key windows and reports structural statistics.
*/
type CrossSection struct {
	mu         sync.RWMutex
	latest     map[string]float64
	recent     map[string][]float64
	members    map[string]*memberState
	estimators map[string]*aggregateEstimator
	lastAt     time.Time
}

/*
NewCrossSection constructs an empty generic cross-section.
*/
func NewCrossSection() *CrossSection {
	return &CrossSection{
		latest:     make(map[string]float64),
		recent:     make(map[string][]float64),
		members:    make(map[string]*memberState),
		estimators: make(map[string]*aggregateEstimator),
	}
}

/*
Process folds one value for one key into the cross-section and emits a generic
Snapshot. The focal key, when non-empty, additionally receives the
focal-relative comparison fields. Causal estimator views are computed before
the current values update the estimator state.
*/
func (section *CrossSection) Process(
	key string,
	value float64,
	at time.Time,
	focal string,
) (Snapshot, bool) {
	if !finiteValue(value) {
		return Snapshot{}, false
	}

	section.mu.Lock()
	defer section.mu.Unlock()

	member := section.members[key]

	if member == nil {
		member = &memberState{}
		section.members[key] = member
	}

	previous := member.latest
	member.previous = previous
	member.latest = value

	if member.at.IsZero() {
		member.from = at
	}

	member.at = at
	member.observations++
	section.latest[key] = value

	if !previousHasValue(previous) || previous == 0 {
		return Snapshot{}, false
	}

	relative := (value - previous) / previous
	section.recent[key] = append(section.recent[key], relative)

	if len(section.recent[key]) > crossSectionPathLength {
		section.recent[key] = section.recent[key][1:]
	}

	snapshot := section.snapshot(at, focal)
	section.updateEstimators(snapshot, at)
	section.lastAt = at

	return snapshot, true
}

func previousHasValue(value float64) bool {
	return !math.IsNaN(value)
}

/*
Snapshot aggregates every member's latest relative change.
*/
func (section *CrossSection) snapshot(at time.Time, focal string) Snapshot {
	values := make([]float64, 0, len(section.recent))
	magnitudes := make([]float64, 0, len(section.recent))
	ages := make([]float64, 0, len(section.recent))
	fromAges := make([]float64, 0, len(section.recent))
	snapshot := Snapshot{At: at, Aggregates: map[string]AggregateView{}}

	for key, recentValues := range section.recent {
		if len(recentValues) == 0 {
			continue
		}

		relative := recentValues[len(recentValues)-1]
		values = append(values, relative)
		magnitudes = append(magnitudes, math.Abs(relative))
		member := section.members[key]

		if member != nil && !member.at.IsZero() {
			ages = append(ages, at.Sub(member.at).Seconds())
			fromAges = append(fromAges, at.Sub(member.from).Seconds())
		}
	}

	snapshot.Count = len(values)
	snapshot.TotalMembers = len(section.members)

	if snapshot.Count == 0 {
		return snapshot
	}

	sort.Float64s(values)
	sort.Float64s(magnitudes)

	for _, relative := range values {
		if relative > 0 {
			snapshot.PositiveCount++
		} else if relative < 0 {
			snapshot.NegativeCount++
		} else {
			snapshot.ZeroCount++
		}
	}

	snapshot.SignedMedian = medianSorted(values)
	snapshot.MedianAbsolute = medianSorted(magnitudes)
	snapshot.MeanAbsolute = meanValue(magnitudes)
	snapshot.Mad = medianSorted(deviationSorted(values, snapshot.SignedMedian))
	snapshot.MagnitudeMad = medianSorted(
		deviationSorted(magnitudes, snapshot.MedianAbsolute),
	)
	snapshot.Iqr = quantileSorted(magnitudes, 0.75) - quantileSorted(magnitudes, 0.25)
	snapshot.Rms = rmsValue(values)

	extremeMagnitude := 0.0
	extremeSigned := 0.0
	extremeKey := ""
	second := 0.0
	tieCount := 0
	sumMagnitude := 0.0

	for key, recentValues := range section.recent {
		relative := recentValues[len(recentValues)-1]
		sumMagnitude += math.Abs(relative)

		if math.Abs(relative) > extremeMagnitude {
			second = extremeMagnitude
			extremeMagnitude = math.Abs(relative)
			extremeSigned = relative
			extremeKey = key
			tieCount = 0
		} else if math.Abs(relative) == extremeMagnitude {
			tieCount++
		} else if math.Abs(relative) > second {
			second = math.Abs(relative)
		}
	}

	snapshot.ExtremeKey = extremeKey
	snapshot.ExtremeMagnitude = extremeMagnitude
	snapshot.ExtremeSigned = extremeSigned
	snapshot.ExtremeSecond = second
	snapshot.ExtremeTieCount = tieCount
	snapshot.SumMagnitude = sumMagnitude

	snapshot.MaxAge = maxValue(ages)
	snapshot.MeanAge = meanValue(ages)
	snapshot.MedianAge = medianUnsorted(ages)
	snapshot.MedianFromAge = medianUnsorted(fromAges)

	if focal != "" {
		focalMember := section.members[focal]

		if focalMember != nil && !focalMember.at.IsZero() {
			snapshot.FocalAge = at.Sub(focalMember.at).Seconds()
			snapshot.FocalFromAge = at.Sub(focalMember.from).Seconds()
		}

		focalValues := section.recent[focal]

		if len(focalValues) > 0 {
			focalRelative := focalValues[len(focalValues)-1]
			peerValues := make([]float64, 0, len(values)-1)
			peerMagnitudes := make([]float64, 0, len(values)-1)

			for key, recentValues := range section.recent {
				if key == focal {
					continue
				}

				relative := recentValues[len(recentValues)-1]
				peerValues = append(peerValues, relative)
				peerMagnitudes = append(peerMagnitudes, math.Abs(relative))

				switch {
				case focalRelative > 0 && relative > 0, focalRelative < 0 && relative < 0:
					snapshot.SameDirectionCount++
				case focalRelative > 0 && relative < 0, focalRelative < 0 && relative > 0:
					snapshot.OppositeDirectionCount++
				default:
					snapshot.ZeroDirectionCount++
				}
			}

			if len(peerMagnitudes) > 0 {
				snapshot.PeerMedianAbsolute = medianUnsorted(peerMagnitudes)
				peerMedian := snapshot.PeerMedianAbsolute
				peerDeviations := make([]float64, 0, len(peerMagnitudes))

				for _, magnitude := range peerMagnitudes {
					peerDeviations = append(peerDeviations, math.Abs(magnitude-peerMedian))
				}

				snapshot.PeerMad = medianUnsorted(peerDeviations)
			}
		}
	}

	// Structural aggregate series for the causal estimator views.
	valid := float64(snapshot.Count)

	if valid > 0 {
		snapshot.Aggregates["signed_fraction"] = AggregateView{
			Value: float64(snapshot.PositiveCount-snapshot.NegativeCount) / valid,
		}
		snapshot.Aggregates["signed_median"] = AggregateView{Value: snapshot.SignedMedian}
		snapshot.Aggregates["median_absolute"] = AggregateView{Value: snapshot.MedianAbsolute}
		snapshot.Aggregates["mean_absolute"] = AggregateView{Value: snapshot.MeanAbsolute}
		snapshot.Aggregates["rms"] = AggregateView{Value: snapshot.Rms}
		snapshot.Aggregates["iqr"] = AggregateView{Value: snapshot.Iqr}
		snapshot.Aggregates["extreme_magnitude"] = AggregateView{Value: extremeMagnitude}

		if sumMagnitude > 0 {
			snapshot.Aggregates["extreme_share"] = AggregateView{
				Value: extremeMagnitude / sumMagnitude,
			}
		}

		if snapshot.MedianAbsolute > 0 {
			snapshot.Aggregates["extreme_ratio"] = AggregateView{
				Value: extremeMagnitude / snapshot.MedianAbsolute,
			}
		}
	}

	// Overlay each aggregate's causal estimator state computed before this tick.
	for name, view := range snapshot.Aggregates {
		if estimator := section.estimators[name]; estimator != nil && estimator.hasValue {
			view.Baseline = estimator.baseline

			if estimator.energy > 0 {
				view.Divergence = view.Value - estimator.baseline
				view.ZScore = view.Divergence / math.Sqrt(estimator.energy)
			}

			if estimator.lastAt > 0 {
				elapsed := float64(at.UnixNano())/1e9 - estimator.lastAt

				if elapsed > 0 {
					view.Velocity = (view.Value - estimator.lastValue) / elapsed
				}
			}

			view.Ready = true
		}

		snapshot.Aggregates[name] = view
	}

	return snapshot
}

/*
updateEstimators advances each aggregate's causal estimator with the current
snapshot values, after the snapshot has been formed.
*/
func (section *CrossSection) updateEstimators(snapshot Snapshot, at time.Time) {
	for name, view := range snapshot.Aggregates {
		estimator := section.estimators[name]

		if estimator == nil {
			estimator = &aggregateEstimator{}
			section.estimators[name] = estimator
		}

		if !estimator.hasValue {
			estimator.baseline = view.Value
			estimator.energy = 0
			estimator.hasValue = true
		} else {
			alpha := 0.5
			estimator.baseline += alpha * (view.Value - estimator.baseline)
			residual := view.Value - estimator.baseline
			estimator.energy += alpha * (residual*residual - estimator.energy)
		}

		estimator.lastValue = view.Value
		estimator.lastAt = float64(at.UnixNano()) / 1e9
		estimator.observations++
	}
}

func medianSorted(sorted []float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	middle := len(sorted) / 2

	if len(sorted)%2 == 0 {
		return (sorted[middle-1] + sorted[middle]) / 2
	}

	return sorted[middle]
}

func medianUnsorted(values []float64) float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	return medianSorted(sorted)
}

func quantileSorted(sorted []float64, quantile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	position := quantile * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))

	if lower == upper {
		return sorted[lower]
	}

	weight := position - float64(lower)

	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

func deviationSorted(sorted []float64, center float64) []float64 {
	deviations := make([]float64, 0, len(sorted))

	for _, value := range sorted {
		deviations = append(deviations, math.Abs(value-center))
	}

	sort.Float64s(deviations)

	return deviations
}

func meanValue(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	total := 0.0

	for _, value := range values {
		total += value
	}

	return total / float64(len(values))
}

func rmsValue(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	total := 0.0

	for _, value := range values {
		total += value * value
	}

	return math.Sqrt(total / float64(len(values)))
}

func maxValue(values []float64) float64 {
	maximum := 0.0

	for _, value := range values {
		if value > maximum {
			maximum = value
		}
	}

	return maximum
}

func finiteValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
