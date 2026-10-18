package audit

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/system"
)

/*
SympathyStage audits cross-measurement, cross-source asynchronous co-movement.
Unlike intra-measurement checks (which evaluate metrics co-emitted in the same snapshot),
SympathyStage compares asynchronous event paths from different producers or symbols.
It uses the continuous-time Hayashi-Yoshida algorithm to calculate overlap covariance
without forcing data into artificial, distortive time buckets (preventing the Epps effect).
*/
type SympathyStage struct {
	*runtime.System
	ch           chan *data.Value
	violations   []*Violation
	streams      map[string][]temporal.Price
	flaggedPairs map[string]bool
	estimates    []map[string]any
	realCounts   []int
	nullCounts   []int
	nullAbs      []int
	estimator    core.Primitive
}

func NewSympathyStage(ctx context.Context) *SympathyStage {
	return &SympathyStage{
		System:       runtime.NewSystem(ctx, "audit:sympathy"),
		ch:           make(chan *data.Value, 64),
		violations:   make([]*Violation, 0),
		streams:      make(map[string][]temporal.Price),
		flaggedPairs: make(map[string]bool),
		realCounts:   make([]int, sympathyHistBins),
		nullCounts:   make([]int, sympathyHistBins),
		nullAbs:      make([]int, sympathyHistBins),
		estimator:    algo.NewHayashiYoshida(),
	}
}

func (stage *SympathyStage) Channel() chan *data.Value {
	return stage.ch
}

func (stage *SympathyStage) Run() error {
	errnie.Info(fmt.Sprintf("%s: Running audit...", stage.Name()))

	for measurement := range stage.ch {
		if measurement == nil {
			stage.violations = append(stage.violations, NewViolation(
				"unknown",
				"measurement",
				"Nil measurement encountered in tape",
				nil,
				nil,
			))

			continue
		}

		if measurement.Role == "runtime:join" {
			continue
		}

		if measurement.Role == "error" {
			stage.violations = append(stage.violations, NewViolation(
				measurement.Scope,
				"measurement.error",
				"error measurement encountered",
				measurement,
				nil,
			))
		}

		stage.recordMeasurement(measurement)
	}

	stage.evaluateAsynchronousCovariance()
	return nil
}

func (stage *SympathyStage) Violations() []*Violation {
	return stage.violations
}

/*
recordMeasurement appends price points to the respective metric stream timeline.
Each point records the nanosecond venue timestamp (At) and the raw numeric value.
*/
func (stage *SympathyStage) recordMeasurement(measurement *data.Value) {
	// Timestamp is the tick's local receive time in nanoseconds.
	venueTime := int64(measurement.Timestamp)

	for label, val := range floatMetrics(measurement) {
		streamKey := measurement.Scope + ":" + measurement.Origin + ":" + label
		stage.streams[streamKey] = append(stage.streams[streamKey], temporal.Price{
			At:    venueTime,
			Value: val,
		})
	}
}

/*
buildReturnPath constructs strictly advancing continuous-time intervals and increments,
handling non-positive metrics and identical venue timestamps honestly without faking timestamps.
*/
func buildReturnPath(prices []temporal.Price) (temporal.ReturnPath, bool) {
	if len(prices) < 2 {
		return temporal.ReturnPath{}, false
	}

	sortedPrices := make([]temporal.Price, len(prices))
	copy(sortedPrices, prices)

	slices.SortStableFunc(sortedPrices, func(first, second temporal.Price) int {
		if first.At < second.At {
			return -1
		}

		if first.At > second.At {
			return 1
		}

		return 0
	})

	deduped := make([]temporal.Price, 0, len(sortedPrices))

	for _, pricePoint := range sortedPrices {
		if len(deduped) > 0 && deduped[len(deduped)-1].At == pricePoint.At {
			deduped[len(deduped)-1].Value = pricePoint.Value
			continue
		}

		deduped = append(deduped, pricePoint)
	}

	if len(deduped) < 2 {
		return temporal.ReturnPath{}, false
	}

	returns := make([]temporal.LogReturn, 0, len(deduped)-1)
	var energy float64
	lastAt := deduped[0].At
	prevVal := deduped[0].Value

	for idx := 1; idx < len(deduped); idx++ {
		curr := deduped[idx]
		venueTime := curr.At

		var increment float64

		if prevVal > 0 && curr.Value > 0 {
			increment = math.Log(curr.Value / prevVal)
		}

		if prevVal <= 0 || curr.Value <= 0 {
			increment = curr.Value - prevVal
		}

		returns = append(returns, temporal.LogReturn{
			From:  lastAt,
			To:    venueTime,
			Value: increment,
		})

		energy += increment * increment
		lastAt = venueTime
		prevVal = curr.Value
	}

	if energy <= 0 || len(returns) == 0 {
		return temporal.ReturnPath{}, false
	}

	return temporal.ReturnPath{
		Returns: returns,
		Energy:  energy,
	}, true
}

/*
evaluateAsynchronousCovariance decodes return paths for all tracked streams and
runs the Hayashi-Yoshida algorithm over stream pairs to detect cross-source redundancy.
*/
func (stage *SympathyStage) evaluateAsynchronousCovariance() {
	streamKeys := make([]string, 0, len(stage.streams))

	for streamKey, prices := range stage.streams {
		if len(prices) >= int(core.MinimumRankPrior) {
			streamKeys = append(streamKeys, streamKey)
		}
	}

	slices.Sort(streamKeys)

	// Decode return paths (intervals [From, To] and cumulative return energy) for eligible streams.
	returnPaths := make(map[string]temporal.ReturnPath, len(streamKeys))

	for _, streamKey := range streamKeys {
		prices := stage.streams[streamKey]
		returnPath, ok := buildReturnPath(prices)

		if ok {
			returnPaths[streamKey] = returnPath
		}
	}

	// Evaluate Hayashi-Yoshida asynchronous covariance across pairs of different streams.
	for outerIdx := 0; outerIdx < len(streamKeys); outerIdx++ {
		for innerIdx := outerIdx + 1; innerIdx < len(streamKeys); innerIdx++ {
			keyA := streamKeys[outerIdx]
			keyB := streamKeys[innerIdx]
			pairKey := keyA + "<->" + keyB

			leftPath, leftOk := returnPaths[keyA]
			rightPath, rightOk := returnPaths[keyB]

			if !leftOk || !rightOk {
				continue
			}

			input := correlation.EstimateInput{
				Left:        leftPath.Returns,
				Right:       rightPath.Returns,
				LeftEnergy:  leftPath.Energy,
				RightEnergy: rightPath.Energy,
				Lag:         0,
			}

			// HayashiYoshida checks continuous-time overlap between [From, To] intervals
			// of left and right, computing covariance and its plug-in null standard error.
			estimate := transport.To[correlation.EstimateInput, correlation.LagEstimate](
				stage.estimator,
				&input,
			)

			// Standardized Hayashi-Yoshida concordance: Score / sqrt(Support) scales into [-1, 1]
			// by Cauchy-Schwarz inequality.
			// Two different assets are allowed to move independently. A failure is a
			// second copy of a stream: |concordance| at the co-movement bar, the
			// score past the significance bound, and the shuffled null not reaching it.
			permutations := system.Cfg.Audit.Permutations
			significance := system.Cfg.Audit.Significance

			bound := 1.0 / math.Sqrt(significance)
			concordance := estimate.Score / math.Sqrt(estimate.Support)

			if estimate.ScoreDefined && estimate.Support > 0 {
				pValue := 1.0
				addHistogram(stage.realCounts, -1, 1, concordance)

				if len(rightPath.Returns) > 1 {
					exceedCount := 0
					shuffledReturns := make([]temporal.LogReturn, len(rightPath.Returns))
					copy(shuffledReturns, rightPath.Returns)
					rng := rand.New(rand.NewSource(42 + int64(outerIdx*1000+innerIdx)))

					for range permutations {
						rng.Shuffle(len(shuffledReturns), func(i, j int) {
							shuffledReturns[i].Value, shuffledReturns[j].Value = shuffledReturns[j].Value, shuffledReturns[i].Value
						})

						for i := range shuffledReturns {
							if rng.Intn(2) == 1 {
								shuffledReturns[i].Value = -shuffledReturns[i].Value
							}
						}

						nullInput := correlation.EstimateInput{
							Left:        leftPath.Returns,
							Right:       shuffledReturns,
							LeftEnergy:  leftPath.Energy,
							RightEnergy: rightPath.Energy,
							Lag:         0,
						}

						nullEstimate := transport.To[correlation.EstimateInput, correlation.LagEstimate](
							stage.estimator,
							&nullInput,
						)

						if nullEstimate.ScoreDefined && nullEstimate.Support > 0 {
							nullConcordance := nullEstimate.Score / math.Sqrt(nullEstimate.Support)
							addHistogram(stage.nullCounts, -1, 1, nullConcordance)
							addHistogram(stage.nullAbs, 0, 1, math.Abs(nullConcordance))

							if math.Abs(nullConcordance) >= math.Abs(concordance) {
								exceedCount++
							}
						}
					}

					pValue = float64(1+exceedCount) / float64(permutations+1)
				}

				partsA := strings.SplitN(keyA, ":", 3)
				partsB := strings.SplitN(keyB, ":", 3)
				isCrossAsset := len(partsA) == 3 && len(partsB) == 3 && partsA[0] != partsB[0]

				isRedundant := math.Abs(concordance) >= system.Cfg.Audit.ComovementConcordance && math.Abs(estimate.Score) >= bound && pValue <= significance

				stage.estimates = append(stage.estimates, map[string]any{
					"pair":           pairKey,
					"concordance":    concordance,
					"score":          estimate.Score,
					"support":        estimate.Support,
					"p_value":        pValue,
					"permutations":   permutations,
					"redundant":      isRedundant,
					"is_cross_asset": isCrossAsset,
				})

				if isRedundant && !stage.flaggedPairs[pairKey] {
					stage.flaggedPairs[pairKey] = true
					stage.violations = append(stage.violations, NewViolation(
						"cross_stream",
						"sympathy.cross_stream_redundancy",
						fmt.Sprintf(
							"Asynchronous stream pair %s shows extreme continuous-time co-movement (concordance = %.4f >= %.2f, z-score = %.2f, p-value = %.4f vs shuffled null, N=%d)",
							pairKey, concordance, system.Cfg.Audit.ComovementConcordance, estimate.Score, pValue, permutations,
						),
						nil,
						nil,
					))
				}
			}
		}
	}
}

func (stage *SympathyStage) Report() string {
	var build strings.Builder

	for _, violation := range stage.violations {
		build.WriteString(violation.String())
	}

	return build.String()
}

const sympathyHistBins = 25

func (stage *SympathyStage) Data() any {
	edges := histogramEdges(sympathyHistBins, -1, 1)

	return map[string]any{
		"passed":          len(stage.violations) == 0,
		"total_streams":   len(stage.streams),
		"redundant_count": len(stage.flaggedPairs),
		"estimates":       stage.estimates,
		"real_distribution": map[string]any{
			"histogram_bins":   edges,
			"histogram_counts": stage.realCounts,
		},
		"null_distribution": map[string]any{
			"histogram_bins":   edges,
			"histogram_counts": stage.nullCounts,
			"percentile_95":    histogramQuantile(stage.nullAbs, 0, 1, 0.95),
		},
	}
}

func histogramEdges(bins int, low, high float64) []float64 {
	edges := make([]float64, bins+1)
	step := (high - low) / float64(bins)

	for i := range edges {
		edges[i] = low + float64(i)*step
	}

	edges[len(edges)-1] = high
	return edges
}

func addHistogram(counts []int, low, high, value float64) {
	if len(counts) == 0 {
		return
	}

	if value < low {
		value = low
	}

	if value > high {
		value = high
	}

	step := (high - low) / float64(len(counts))
	idx := int((value - low) / step)

	if idx >= len(counts) {
		idx = len(counts) - 1
	}

	if idx < 0 {
		idx = 0
	}

	counts[idx]++
}

func histogramQuantile(counts []int, low, high, q float64) float64 {
	total := 0

	for _, count := range counts {
		total += count
	}

	if total == 0 {
		return 0
	}

	target := q * float64(total)
	step := (high - low) / float64(len(counts))
	accumulated := 0

	for i, count := range counts {
		accumulated += count

		if float64(accumulated) >= target {
			return low + float64(i+1)*step
		}
	}

	return high
}
