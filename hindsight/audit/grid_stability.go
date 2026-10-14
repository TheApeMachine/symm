package audit

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
StabilityStage audits whether the market feature grid is stationary across time
and whether grid cell observations are complete across ticks.
It divides observations chronologically into two halves and computes the
Jensen-Shannon Divergence (JSD) between their regional activation distributions.
*/
type StabilityStage struct {
	*runtime.System
	ch          chan *data.Measurement
	violations  []*Violation
	activations []string
	totalFrames int64
	validFrames int64
	jsd         float64
	tvd         float64
}

func NewStabilityStage(ctx context.Context) *StabilityStage {
	return &StabilityStage{
		System:      runtime.NewSystem(ctx, "audit:stability"),
		ch:          make(chan *data.Measurement, 64),
		violations:  make([]*Violation, 0),
		activations: make([]string, 0),
	}
}

func (stage *StabilityStage) Channel() chan *data.Measurement {
	return stage.ch
}

func (stage *StabilityStage) Run() error {
	errnie.Info(fmt.Sprintf("%s: Running audit...", stage.Name()))

	for measurement := range stage.ch {
		stage.totalFrames++

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

		if measurement.Error() != nil {
			stage.violations = append(stage.violations, NewViolation(
				measurement.Label,
				"measurement.error",
				measurement.Error().Error(),
				measurement,
				nil,
			))
		}

		stage.recordFrame(measurement)
	}

	stage.auditStationarity()
	return nil
}

func (stage *StabilityStage) Violations() []*Violation {
	return stage.violations
}

/*
recordFrame logs the dominant metric or regional token for the arrival.
If the measurement has active metric entries, it counts as a valid observed frame.
*/
func (stage *StabilityStage) recordFrame(measurement *data.Measurement) {
	hasMetrics := false

	for entry := range measurement.Read() {
		if entry != nil && entry.Metric != nil {
			hasMetrics = true
			stage.activations = append(stage.activations, entry.Metric.Label())
			break
		}
	}

	if hasMetrics {
		stage.validFrames++
	}
}

/*
auditStationarity divides the observed activation sequence into halves
and computes Jensen-Shannon Divergence (JSD) and Total Variation Distance (TVD).
*/
func (stage *StabilityStage) auditStationarity() {
	if stage.totalFrames == 0 {
		return
	}

	// 1. Grid Completeness Check: Ensure at least 80% of ticks have valid observations.
	completeness := float64(stage.validFrames) / float64(stage.totalFrames)

	if stage.totalFrames >= 20 && completeness < 0.80 {
		stage.violations = append(stage.violations, NewViolation(
			"grid",
			"grid.incomplete",
			fmt.Sprintf("Grid completeness below threshold: %.1f%% (%d of %d frames)",
				completeness*100, stage.validFrames, stage.totalFrames),
			nil,
			nil,
		))
	}

	// 2. Stationarity Check: Need enough observations to divide into two halves.
	totalActivations := len(stage.activations)

	if totalActivations < 20 {
		return
	}

	half := totalActivations / 2
	firstHalf := stage.activations[:half]
	secondHalf := stage.activations[half:]

	distFirst := empiricalDistribution(firstHalf)
	distSecond := empiricalDistribution(secondHalf)

	stage.jsd, stage.tvd = computeJSD(distFirst, distSecond)
	jsd, tvd := stage.jsd, stage.tvd

	// JSD is bounded in [0, 1] bits. A JSD > 0.40 bits indicates a severe,
	// non-stationary regime shift between the first and second half of the audit window.
	if jsd > 0.40 {
		stage.violations = append(stage.violations, NewViolation(
			"grid",
			"grid.non_stationary",
			fmt.Sprintf("Grid excitation non-stationary: JSD = %.3f bits > 0.40 bound (TVD = %.3f)",
				jsd, tvd),
			nil,
			nil,
		))
	}
}

func empiricalDistribution(tokens []string) map[string]float64 {
	counts := make(map[string]float64)

	for _, token := range tokens {
		counts[token]++
	}

	total := float64(len(tokens))
	dist := make(map[string]float64, len(counts))

	for token, count := range counts {
		dist[token] = count / total
	}

	return dist
}

/*
computeJSD calculates Jensen-Shannon Divergence (in bits, log2) and Total Variation Distance
between two discrete probability distributions P and Q.
M = 0.5 * (P + Q)
JSD(P || Q) = 0.5 * KL(P || M) + 0.5 * KL(Q || M)
*/
func computeJSD(distP, distQ map[string]float64) (float64, float64) {
	allKeys := make(map[string]bool)

	for key := range distP {
		allKeys[key] = true
	}

	for key := range distQ {
		allKeys[key] = true
	}

	var klP, klQ, tvd float64

	for key := range allKeys {
		probP := distP[key]
		probQ := distQ[key]
		meanM := 0.5 * (probP + probQ)

		if probP > 0 && meanM > 0 {
			klP += probP * math.Log2(probP/meanM)
		}

		if probQ > 0 && meanM > 0 {
			klQ += probQ * math.Log2(probQ/meanM)
		}

		tvd += 0.5 * math.Abs(probP-probQ)
	}

	jsd := 0.5*klP + 0.5*klQ

	return jsd, tvd
}

func (stage *StabilityStage) Report() string {
	var build strings.Builder

	for _, violation := range stage.violations {
		build.WriteString(violation.String())
	}

	return build.String()
}

func (stage *StabilityStage) Data() any {
	completeness := 0.0

	if stage.totalFrames > 0 {
		completeness = float64(stage.validFrames) / float64(stage.totalFrames)
	}

	return map[string]any{
		"passed":       len(stage.violations) == 0,
		"total_frames": stage.totalFrames,
		"valid_frames": stage.validFrames,
		"completeness": completeness,
		"jsd":          stage.jsd,
		"tvd":          stage.tvd,
	}
}

