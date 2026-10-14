package audit

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/temporal"
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
	ch           chan *data.Measurement
	violations   []*Violation
	streams      map[string][]temporal.Price
	flaggedPairs map[string]bool
	estimates    []map[string]any
	estimator    core.Primitive
}

func NewSympathyStage(ctx context.Context) *SympathyStage {
	return &SympathyStage{
		System:       runtime.NewSystem(ctx, "audit:sympathy"),
		ch:           make(chan *data.Measurement, 64),
		violations:   make([]*Violation, 0),
		streams:      make(map[string][]temporal.Price),
		flaggedPairs: make(map[string]bool),
		estimator:    algo.NewHayashiYoshida(),
	}
}

func (stage *SympathyStage) Channel() chan *data.Measurement {
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

		if measurement.Source == "runtime:join" {
			continue
		}

		if measurement.Error() != nil {
			stage.violations = append(stage.violations, NewViolation(
				measurement.Label,
				"measurement.error",
				measurement.Error().Error(),
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
func (stage *SympathyStage) recordMeasurement(measurement *data.Measurement) {
	if measurement.At.IsZero() {
		return
	}

	venueTime := measurement.At.UnixNano()

	for entry := range measurement.Read() {
		if entry == nil || entry.Metric == nil {
			stage.violations = append(stage.violations, NewViolation(
				measurement.Label,
				"entry.metric",
				"Nil metric entry encountered in measurement",
				measurement,
				nil,
			))

			continue
		}

		streamKey := measurement.Source + "|" + entry.Metric.Label()
		stage.streams[streamKey] = append(stage.streams[streamKey], temporal.Price{
			At:    venueTime,
			Value: entry.Metric.Raw(),
		})
	}
}

/*
buildReturnPath constructs strictly advancing continuous-time intervals and increments,
handling non-positive metrics and identical venue timestamps safely.
*/
func buildReturnPath(prices []temporal.Price) (temporal.ReturnPath, bool) {
	if len(prices) < 2 {
		return temporal.ReturnPath{}, false
	}

	returns := make([]temporal.LogReturn, 0, len(prices)-1)
	var energy float64
	lastAt := prices[0].At
	prevVal := prices[0].Value

	for idx := 1; idx < len(prices); idx++ {
		curr := prices[idx]
		venueTime := curr.At

		if venueTime <= lastAt {
			venueTime = lastAt + 1
		}

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
			estimate := data.To[correlation.EstimateInput, correlation.LagEstimate](
				stage.estimator,
				&input,
			)

			// Standardized Hayashi-Yoshida concordance: Score / sqrt(Support) scales into [-1, 1]
			// by Cauchy-Schwarz inequality.
			// When |concordance| >= 0.95 (VIF > 10) and Score exceeds the significance bound,
			// the two asynchronous streams are collinear clones.
			significance := system.Cfg.Audit.Significance

			if significance <= 0 || significance >= 1 {
				significance = 0.05
			}

			bound := 1.0 / math.Sqrt(significance)
			concordance := estimate.Score / math.Sqrt(estimate.Support)

			if estimate.ScoreDefined && estimate.Support > 0 {
				isRedundant := math.Abs(concordance) >= 0.95 && math.Abs(estimate.Score) >= bound

				stage.estimates = append(stage.estimates, map[string]any{
					"pair":        pairKey,
					"concordance": concordance,
					"score":       estimate.Score,
					"support":     estimate.Support,
					"redundant":   isRedundant,
				})

				if isRedundant && !stage.flaggedPairs[pairKey] {
					stage.flaggedPairs[pairKey] = true
					stage.violations = append(stage.violations, NewViolation(
						"cross_stream",
						"sympathy.cross_stream_redundancy",
						fmt.Sprintf(
							"Asynchronous stream pair %s shows extreme continuous-time co-movement (concordance = %.4f >= 0.95, z-score = %.2f)",
							pairKey, concordance, estimate.Score,
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

