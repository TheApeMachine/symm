package audit

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
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
	ch            chan *data.Value
	violations    []*Violation
	grid          *nmstore.Grid
	activations   []string
	totalFrames   int64
	validFrames   int64
	warmingFrames int64
	err           error
	jsd           float64
	tvd           float64
	nullJSDMean   float64
	nullJSDP95    float64
	pValue        float64
	permutations  int
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
		if stage.err != nil {
			continue
		}

		if measurement == nil {
			stage.err = errnie.Err(errnie.Validation, stage.Name()+": nil measurement on the tape", nil)
			continue
		}

		if measurement.Role != "runtime:join" {
			continue
		}

		stage.recordFrame(measurement)
	}

	if stage.err != nil {
		return stage.err
	}

	stage.auditStationarity()
	return nil
}

func (stage *StabilityStage) Violations() []*Violation {
	return stage.violations
}

/*
recordFrame logs the dominant region of one symbol's frame. Observe reads
the signal layers already on the value.
*/
func (stage *StabilityStage) recordFrame(measurement *data.Value) {
	symbol := measurement.Scope
	signals := make([]broker.Sample, 0)

	for signal, err := range broker.Samples(measurement) {
		if err != nil {
			stage.grid.Error(err)
			continue
		}

		if signal.Symbol != symbol {
			continue
		}

		signals = append(signals, signal)
	}

	if err := stage.grid.Error(); err != nil {
		stage.err = err
		return
	}

	regions, err := stage.grid.Observe(symbol, signals)

	if err != nil {
		stage.err = err
		return
	}

	stage.totalFrames++

	if len(regions) == 0 {
		stage.warmingFrames++
		return
	}

	if regions[0].Brightness <= 0 {
		return
	}

	stage.activations = append(stage.activations, regionToken(regions[0].Key))
	stage.validFrames++
}

/*
auditStationarity divides the observed activation sequence into halves
and computes Jensen-Shannon Divergence (JSD) and Total Variation Distance (TVD).
*/
func (stage *StabilityStage) auditStationarity() {
	cfg := system.Cfg.Audit
	scorable := stage.totalFrames - stage.warmingFrames

	if scorable < int64(cfg.MinTokens) || len(stage.activations) < cfg.MinTokens {
		stage.violations = append(stage.violations, insufficient("grid", fmt.Sprintf(
			"%d scorable frames (%d warming up) and %d activations; need %d of each",
			scorable, stage.warmingFrames, len(stage.activations), cfg.MinTokens,
		)))
		return
	}

	// 1. Grid completeness: scorable frames that light a region.
	completeness := float64(stage.validFrames) / float64(scorable)

	if completeness < cfg.GridCompleteness {
		stage.violations = append(stage.violations, NewViolation(
			"grid",
			"grid.incomplete",
			fmt.Sprintf("Grid completeness %.1f%% below %.1f%% (%d of %d scorable frames, %d warming up)",
				completeness*100, cfg.GridCompleteness*100, stage.validFrames, scorable, stage.warmingFrames),
			nil,
			nil,
		))
	}

	// 2. Stationarity: first versus second half of the activations.
	mid := len(stage.activations) / 2
	dist1 := computeFrequencyDistribution(stage.activations[:mid])
	dist2 := computeFrequencyDistribution(stage.activations[mid:])

	stage.jsd = computeJSD(dist1, dist2)
	stage.tvd = computeTVD(dist1, dist2)
	stage.permutations = cfg.Permutations
	stage.pValue, stage.nullJSDMean, stage.nullJSDP95 = permutationNull(
		stage.activations, mid, stage.jsd, stage.permutations, tokenJSD,
	)

	if stage.jsd > cfg.StationarityJSD && stage.pValue < cfg.Significance {
		stage.violations = append(stage.violations, NewViolation(
			"grid",
			"grid.non_stationary",
			fmt.Sprintf("Feature grid regional activation is non-stationary across time halves: JSD = %.4f bits (threshold: %.2f), TVD = %.4f, p-value = %.4f (null 95th pct: %.4f)",
				stage.jsd, cfg.StationarityJSD, stage.tvd, stage.pValue, stage.nullJSDP95),
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
		"warming_frames":     stage.warmingFrames,
		"completeness":       float64(stage.validFrames) / math.Max(1.0, float64(stage.totalFrames-stage.warmingFrames)),
		"jsd_bits":           stage.jsd,
		"tvd":                stage.tvd,
		"p_value":            stage.pValue,
		"null_jsd_mean_bits": stage.nullJSDMean,
		"null_jsd_p95_bits":  stage.nullJSDP95,
		"permutations":       stage.permutations,
	}
}
