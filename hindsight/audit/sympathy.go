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
	estimator    core.Primitive
}

func NewSympathyStage(ctx context.Context) *SympathyStage {
	return &SympathyStage{
		System:       runtime.NewSystem(ctx, "audit:sympathy"),
		ch:           make(chan *data.Value, 64),
		violations:   make([]*Violation, 0),
		streams:      make(map[string][]temporal.Price),
		flaggedPairs: make(map[string]bool),
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

	for label, val := range data.Metrics(measurement) {
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
			// When |concordance| >= 0.95 (VIF > 10) and Score exceeds the significance bound,
			// the two asynchronous streams are candidates for collinear redundancy.
			// We test statistical significance against a shuffled null using system.Cfg.Audit.Permutations.
			permutations := system.Cfg.Audit.Permutations
			significance := system.Cfg.Audit.Significance

			bound := 1.0 / math.Sqrt(significance)
			concordance := estimate.Score / math.Sqrt(estimate.Support)

			if estimate.ScoreDefined && estimate.Support > 0 {
				pValue := 1.0

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
				isSameMetricStream := isCrossAsset && partsA[2] == partsB[2]

				isRedundant := math.Abs(concordance) >= system.Cfg.Audit.ComovementConcordance && math.Abs(estimate.Score) >= bound && pValue <= significance
				sympathyFailed := isSameMetricStream && (concordance <= 0 || pValue > significance)

				stage.estimates = append(stage.estimates, map[string]any{
					"pair":            pairKey,
					"concordance":     concordance,
					"score":           estimate.Score,
					"support":         estimate.Support,
					"p_value":         pValue,
					"permutations":    permutations,
					"redundant":       isRedundant,
					"is_cross_asset":  isCrossAsset,
					"sympathy_failed": sympathyFailed,
				})

				if sympathyFailed && !stage.flaggedPairs[pairKey] {
					stage.flaggedPairs[pairKey] = true
					stage.violations = append(stage.violations, NewViolation(
						"cross_asset",
						"sympathy.no_concordance",
						fmt.Sprintf(
							"Cross-asset pair %s fails sympathy test: concordance = %.4f <= 0 or p-value = %.4f > %.2f vs shuffled null (z-score = %.2f, N=%d)",
							pairKey, concordance, pValue, significance, estimate.Score, permutations,
						),
						nil,
						nil,
					))
				}

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

func (stage *SympathyStage) Data() any {
	return map[string]any{
		"passed":          len(stage.violations) == 0,
		"total_streams":   len(stage.streams),
		"redundant_count": len(stage.flaggedPairs),
		"estimates":       stage.estimates,
	}
}
