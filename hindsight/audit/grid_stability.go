package audit

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/system"
)

/*
StabilityStage audits whether the market feature grid is stationary across time
and whether grid cell observations are complete across ticks.
It divides observations chronologically into two halves and computes the
Jensen-Shannon Divergence (JSD) between their regional activation distributions.
*/
type StabilityStage struct {
	*runtime.System
	ch           chan *data.Measurement
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
		ch:          make(chan *data.Measurement, 64),
		violations:  make([]*Violation, 0),
		grid:        nmstore.NewGrid(ctx),
		activations: make([]string, 0),
	}
}

func (stage *StabilityStage) Channel() chan *data.Measurement {
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

		if measurement.Source != "runtime:join" {
			continue
		}

		stage.totalFrames++

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
recordFrame logs the dominant regional token for the arrival using Grid.Observe.
*/
func (stage *StabilityStage) recordFrame(measurement *data.Measurement) {
	if len(measurement.Peers()) == 0 {
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

	half := totalActivations / 2
	firstHalf := stage.activations[:half]
	secondHalf := stage.activations[half:]

	distFirst := empiricalDistribution(firstHalf)
	distSecond := empiricalDistribution(secondHalf)

	stage.jsd, stage.tvd = computeJSD(distFirst, distSecond)
	jsd, tvd := stage.jsd, stage.tvd

	permutations := system.Cfg.Audit.Permutations

	if permutations <= 0 {
		permutations = 50
	}

	// Permutation test against tick-shuffled null:
	// Under the null hypothesis that activations are temporally stationary,
	// the chronological order of tick activations is exchangeable.
	exceedCount := 0
	nullJSDs := make([]float64, 0, permutations)
	shuffled := make([]string, totalActivations)
	copy(shuffled, stage.activations)
	rng := rand.New(rand.NewSource(1337))

	for iter := 0; iter < permutations; iter++ {
		rng.Shuffle(totalActivations, func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		nullFirst := empiricalDistribution(shuffled[:half])
		nullSecond := empiricalDistribution(shuffled[half:])
		nullJSD, _ := computeJSD(nullFirst, nullSecond)
		nullJSDs = append(nullJSDs, nullJSD)

		if nullJSD >= jsd {
			exceedCount++
		}
	}

	var sumNull float64

	for _, v := range nullJSDs {
		sumNull += v
	}

	nullMean := sumNull / float64(permutations)
	slices.Sort(nullJSDs)
	p95Idx := int(0.95 * float64(permutations))

	if p95Idx >= len(nullJSDs) {
		p95Idx = len(nullJSDs) - 1
	}

	nullP95 := nullJSDs[p95Idx]
	pValue := float64(1+exceedCount) / float64(permutations+1)

	stage.nullJSDMean = nullMean
	stage.nullJSDP95 = nullP95
	stage.pValue = pValue
	stage.permutations = permutations

	// JSD is bounded in [0, 1] bits. A JSD > 0.40 bits indicates a severe,
	// non-stationary regime shift between the first and second half of the audit window.
	// We verify that the shift is statistically significant against the tick-shuffled null.
	if jsd > 0.40 && (jsd > nullP95 || pValue <= 0.05) {
		stage.violations = append(stage.violations, NewViolation(
			"grid",
			"grid.non_stationary",
			fmt.Sprintf("Grid excitation non-stationary: JSD = %.3f bits > 0.40 bound (shuffled null mean = %.3f, p95 = %.3f, p-value = %.4f, TVD = %.3f)",
				jsd, nullMean, nullP95, pValue, tvd),
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
		"passed":        len(stage.violations) == 0,
		"total_frames":  stage.totalFrames,
		"valid_frames":  stage.validFrames,
		"completeness":  completeness,
		"jsd":           stage.jsd,
		"tvd":           stage.tvd,
		"null_jsd_mean": stage.nullJSDMean,
		"null_jsd_p95":  stage.nullJSDP95,
		"p_value":       stage.pValue,
		"permutations":  stage.permutations,
	}
}

