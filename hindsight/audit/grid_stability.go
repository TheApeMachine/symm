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
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
)

/*
StabilityStage audits whether the market feature grid is stationary across time
and whether grid cell observations are complete across ticks.
It divides observations chronologically into two halves and computes the
Jensen-Shannon Divergence (JSD) between their regional activation distributions.
*/
type StabilityStage struct {
	*runtime.System
	ch           chan *data.Value
	violations   []*Violation
	grid         *nmstore.Grid
	activations  []string
	totalFrames  int64
	validFrames  int64
	jsd          float64
	tvd          float64
	nullJSDMean  float64
	nullJSDP95   float64
	pValue       float64
	permutations int
}

func NewStabilityStage(ctx context.Context) *StabilityStage {
	return &StabilityStage{
		System:      runtime.NewSystem(ctx, "audit:stability"),
		ch:          make(chan *data.Value, 64),
		violations:  make([]*Violation, 0),
		grid:        nmstore.NewGrid(ctx),
		activations: make([]string, 0),
	}
}

func (stage *StabilityStage) Channel() chan *data.Value {
	return stage.ch
}

func (stage *StabilityStage) Run() error {
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

		if measurement.Role != "runtime:join" {
			continue
		}

		stage.totalFrames++

		stage.recordFrame(measurement)
	}

	stage.auditStationarity()
	return nil
}

func (stage *StabilityStage) Violations() []*Violation {
	return stage.violations
}

/*
recordFrame logs the dominant regional token for the arrival using Grid.Observe.
*/
func (stage *StabilityStage) recordFrame(measurement *data.Value) {
	if len(measurement.Layers) == 0 {
		return
	}

	regions := stage.grid.Observe(measurement)

	if len(regions) == 0 || regions[0].Brightness <= 0 {
		return
	}

	token := fmt.Sprintf("R%02d", regions[0].Key)
	stage.activations = append(stage.activations, token)
	stage.validFrames++
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

	mid := totalActivations / 2
	firstHalf := stage.activations[:mid]
	secondHalf := stage.activations[mid:]

	dist1 := computeFrequencyDistribution(firstHalf)
	dist2 := computeFrequencyDistribution(secondHalf)

	stage.jsd = computeJSD(dist1, dist2)
	stage.tvd = computeTVD(dist1, dist2)

	// 3. Permutation Test for Empirical Significance (P-Value & Null P95)
	stage.permutations = 500
	stage.pValue, stage.nullJSDMean, stage.nullJSDP95 = stage.permutationTest(
		stage.activations, mid, stage.jsd, stage.permutations,
	)

	// Violation Criteria:
	// A distribution shift is a non-stationary rupture if JSD > 0.35 bits
	// AND the observed divergence is statistically significant (p-value < 0.05).
	threshold := 0.35

	if stage.jsd > threshold && stage.pValue < 0.05 {
		stage.violations = append(stage.violations, NewViolation(
			"grid",
			"grid.non_stationary",
			fmt.Sprintf("Feature grid regional activation is non-stationary across time halves: JSD = %.4f bits (threshold: %.2f), TVD = %.4f, p-value = %.4f (null 95th pct: %.4f)",
				stage.jsd, threshold, stage.tvd, stage.pValue, stage.nullJSDP95),
			nil,
			nil,
		))
	}
}

func computeFrequencyDistribution(tokens []string) map[string]float64 {
	counts := make(map[string]float64)
	total := float64(len(tokens))

	if total == 0 {
		return counts
	}

	for _, token := range tokens {
		counts[token]++
	}

	for token := range counts {
		counts[token] /= total
	}

	return counts
}

/*
computeJSD calculates Jensen-Shannon Divergence in bits (using base-2 logarithm).
Bounded in [0, 1].
*/
func computeJSD(p, q map[string]float64) float64 {
	allKeys := make(map[string]bool)
	for k := range p {
		allKeys[k] = true
	}
	for k := range q {
		allKeys[k] = true
	}

	m := make(map[string]float64)
	for k := range allKeys {
		m[k] = 0.5 * (p[k] + q[k])
	}

	klPM := computeKLDivergence(p, m)
	klQM := computeKLDivergence(q, m)

	return 0.5*klPM + 0.5*klQM
}

func computeKLDivergence(p, q map[string]float64) float64 {
	var kl float64

	for k, probP := range p {
		if probP <= 0 {
			continue
		}

		probQ := q[k]
		if probQ <= 0 {
			continue
		}

		kl += probP * math.Log2(probP/probQ)
	}

	return kl
}

func computeTVD(p, q map[string]float64) float64 {
	allKeys := make(map[string]bool)
	for k := range p {
		allKeys[k] = true
	}
	for k := range q {
		allKeys[k] = true
	}

	var sumAbs float64
	for k := range allKeys {
		sumAbs += math.Abs(p[k] - q[k])
	}

	return 0.5 * sumAbs
}

func (stage *StabilityStage) permutationTest(
	tokens []string, mid int, observedJSD float64, runs int,
) (float64, float64, float64) {
	shuffled := slices.Clone(tokens)
	nullJSDs := make([]float64, runs)
	exceedCount := 0

	r := rand.New(rand.NewSource(42))

	for i := range runs {
		r.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		dist1 := computeFrequencyDistribution(shuffled[:mid])
		dist2 := computeFrequencyDistribution(shuffled[mid:])
		jsd := computeJSD(dist1, dist2)
		nullJSDs[i] = jsd

		if jsd >= observedJSD {
			exceedCount++
		}
	}

	slices.Sort(nullJSDs)
	sum := 0.0
	for _, val := range nullJSDs {
		sum += val
	}

	mean := sum / float64(runs)
	p95Idx := int(float64(runs) * 0.95)
	if p95Idx >= runs {
		p95Idx = runs - 1
	}
	p95 := nullJSDs[p95Idx]
	pValue := float64(exceedCount) / float64(runs)

	return pValue, mean, p95
}

func (stage *StabilityStage) Report() string {
	var build strings.Builder

	for _, violation := range stage.violations {
		build.WriteString(violation.String())
	}

	return build.String()
}

func (stage *StabilityStage) Data() any {
	return map[string]any{
		"passed":             len(stage.violations) == 0,
		"total_frames":       stage.totalFrames,
		"valid_frames":       stage.validFrames,
		"completeness":       float64(stage.validFrames) / math.Max(1.0, float64(stage.totalFrames)),
		"jsd_bits":           stage.jsd,
		"tvd":                stage.tvd,
		"p_value":            stage.pValue,
		"null_jsd_mean_bits": stage.nullJSDMean,
		"null_jsd_p95_bits":  stage.nullJSDP95,
		"permutations":       stage.permutations,
	}
}
