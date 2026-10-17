package audit

import (
	"context"
	"fmt"
	"maps"
	"math"
	"math/rand"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

/*
RealizationStage compares, per stored policy path, the edge train predicted
(the policy's gain: mean net return per visit) with the edge paper trading
realized (the mean net_return of the paper records that matched that path
and action under that policy epoch). It reports sample counts, the Pearson
correlation of predicted and realized edge across paths with its
permutation p-value, the share of paths whose signs agree, and the mean
realized-minus-predicted gap.

It passes only when paper data exists, every paper record of the audited
policy epoch names a stored policy, and the correlation is positive and
significant (audit.permutations, audit.significance). No paper records is a
failure: there is nothing to judge the policies by.
*/
type RealizationStage struct {
	*runtime.System
	ch          chan *data.Value
	violations  []*Violation
	predicted   map[string]float64
	epoch       uint64
	realized    map[string][]float64
	pending     []data.PaperRecord
	records     int
	otherEpochs int
	paths       []realizedPath
	correlation float64
	pValue      float64
	signAgree   float64
	meanGap     float64
	gate        *data.PolicyGate
}

/*
realizedPath is one policy path that paper trading traded.
*/
type realizedPath struct {
	Key       string  `json:"key"`
	Samples   int     `json:"samples"`
	Predicted float64 `json:"predicted"`
	Realized  float64 `json:"realized"`
}

func NewRealizationStage(ctx context.Context) *RealizationStage {
	return &RealizationStage{
		System:     runtime.NewSystem(ctx, "audit:realization"),
		ch:         make(chan *data.Value, 64),
		violations: make([]*Violation, 0),
		predicted:  make(map[string]float64),
		realized:   make(map[string][]float64),
	}
}

func (stage *RealizationStage) Channel() chan *data.Value {
	return stage.ch
}

func (stage *RealizationStage) Violations() []*Violation {
	return stage.violations
}

func (stage *RealizationStage) Run() error {
	errnie.Info(fmt.Sprintf("%s: Running audit...", stage.Name()))

	for measurement := range stage.ch {
		if err := stage.observe(measurement); err != nil {
			stage.violations = append(stage.violations, NewViolation(
				"all", "realization.malformed", err.Error(), measurement, nil,
			))
		}
	}

	// Paper records are judged once every policy of the epoch is known.
	for _, record := range stage.pending {
		stage.join(record)
	}

	stage.evaluate()
	return nil
}

func (stage *RealizationStage) observe(measurement *data.Value) error {
	if measurement == nil {
		return fmt.Errorf("nil measurement")
	}

	switch measurement.Origin {
	case data.PolicyOrigin:
		path, err := data.PolicyPath(measurement)

		if err != nil {
			return err
		}

		gain, ok := data.Metrics(measurement)["gain"]

		if !ok {
			return fmt.Errorf("policy %s has no gain", measurement.Prefix)
		}

		if stage.epoch != 0 && stage.epoch != measurement.Epoch {
			return fmt.Errorf("policy %s is from epoch %d, not %d", measurement.Prefix, measurement.Epoch, stage.epoch)
		}

		gate, err := data.ReadPolicyGate(measurement)

		if err != nil {
			return err
		}

		if stage.gate != nil && gate != *stage.gate {
			return fmt.Errorf("policy %s carries %s, other policies %s", measurement.Prefix, gate, stage.gate)
		}

		stage.epoch, stage.gate = measurement.Epoch, &gate
		stage.predicted[measurement.Role+":"+path] = gain
	case data.PaperOrigin:
		record, err := data.PaperFields(measurement)

		if err != nil {
			return err
		}

		stage.pending = append(stage.pending, record)
	default:
		return fmt.Errorf("%s is neither a policy nor a paper record", measurement.Prefix)
	}

	return nil
}

func (stage *RealizationStage) join(record data.PaperRecord) {
	stage.records++

	if record.PolicyEpoch != stage.epoch {
		stage.otherEpochs++
		return
	}

	key := record.Action + ":" + record.Path

	if _, ok := stage.predicted[key]; !ok {
		stage.violations = append(stage.violations, NewViolation(
			record.Symbol, "realization.unknown_path",
			fmt.Sprintf("paper record traded %s under policy epoch %d, which stores no such policy", key, record.PolicyEpoch),
			nil, nil,
		))

		return
	}

	stage.realized[key] = append(stage.realized[key], record.Fields["net_return"])
}

func (stage *RealizationStage) evaluate() {
	if stage.records == 0 {
		stage.violations = append(stage.violations, insufficient(
			"realization", "no paper data: no paper/ records were found (run make run with a model, then make paper)",
		))

		return
	}

	if stage.epoch == 0 {
		stage.violations = append(stage.violations, insufficient("realization", "no policies were published for the audited epoch"))
		return
	}

	for _, key := range slices.Sorted(maps.Keys(stage.realized)) {
		samples := stage.realized[key]
		mean := 0.0

		for _, sample := range samples {
			mean += sample
		}

		stage.paths = append(stage.paths, realizedPath{
			Key: key, Samples: len(samples), Predicted: stage.predicted[key], Realized: mean / float64(len(samples)),
		})
	}

	if len(stage.paths) == 0 {
		stage.violations = append(stage.violations, insufficient("realization", fmt.Sprintf(
			"no paper record traded under policy epoch %d (%d records are from other policy epochs)", stage.epoch, stage.otherEpochs,
		)))

		return
	}

	predicted := make([]float64, len(stage.paths))
	realized := make([]float64, len(stage.paths))
	agree := 0

	for idx, path := range stage.paths {
		predicted[idx], realized[idx] = path.Predicted, path.Realized
		stage.meanGap += path.Realized - path.Predicted

		if math.Signbit(path.Predicted) == math.Signbit(path.Realized) {
			agree++
		}
	}

	stage.meanGap /= float64(len(stage.paths))
	stage.signAgree = float64(agree) / float64(len(stage.paths))

	correlation, ok := pearson(predicted, realized)

	if !ok {
		stage.violations = append(stage.violations, insufficient("realization", fmt.Sprintf(
			"%d traded paths do not vary enough to correlate predicted and realized edge", len(stage.paths),
		)))

		return
	}

	stage.correlation = correlation
	runs := system.Cfg.Audit.Permutations
	shuffled := slices.Clone(realized)
	rng := rand.New(rand.NewSource(17))
	exceed := 0

	for range runs {
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

		if null, _ := pearson(predicted, shuffled); null >= correlation {
			exceed++
		}
	}

	stage.pValue = float64(1+exceed) / float64(runs+1)

	if correlation <= 0 || stage.pValue >= system.Cfg.Audit.Significance {
		stage.violations = append(stage.violations, NewViolation(
			"all", "realization.unpredictive",
			fmt.Sprintf(
				"predicted and realized edge correlate %.4f over %d paths (permutation p %.4f, significance %v)",
				correlation, len(stage.paths), stage.pValue, system.Cfg.Audit.Significance,
			),
			nil, nil,
		))
	}
}

/*
pearson is the correlation of x and y; false when either does not vary.
*/
func pearson(x, y []float64) (float64, bool) {
	n := float64(len(x))

	if len(x) < 2 || len(x) != len(y) {
		return 0, false
	}

	var meanX, meanY float64

	for idx := range x {
		meanX += x[idx]
		meanY += y[idx]
	}

	meanX, meanY = meanX/n, meanY/n
	var cov, varX, varY float64

	for idx := range x {
		cov += (x[idx] - meanX) * (y[idx] - meanY)
		varX += (x[idx] - meanX) * (x[idx] - meanX)
		varY += (y[idx] - meanY) * (y[idx] - meanY)
	}

	if varX == 0 || varY == 0 {
		return 0, false
	}

	return cov / math.Sqrt(varX*varY), true
}

func (stage *RealizationStage) Report() string {
	var build strings.Builder

	build.WriteString(fmt.Sprintf(
		"\npaper records %d (%d under other policy epochs), traded paths %d, correlation %.4f (p %.4f), sign agreement %.3f, mean realized - predicted %.6f\n",
		stage.records, stage.otherEpochs, len(stage.paths), stage.correlation, stage.pValue, stage.signAgree, stage.meanGap,
	))

	for _, path := range stage.paths {
		build.WriteString(fmt.Sprintf("- %s: %d samples, predicted %.6f, realized %.6f\n", path.Key, path.Samples, path.Predicted, path.Realized))
	}

	for _, violation := range stage.violations {
		build.WriteString(violation.String())
	}

	return build.String()
}

func (stage *RealizationStage) Data() any {
	return map[string]any{
		"passed":                        len(stage.violations) == 0,
		"policy_epoch":                  fmt.Sprint(stage.epoch),
		"paper_records":                 stage.records,
		"other_epoch_records":           stage.otherEpochs,
		"traded_paths":                  len(stage.paths),
		"paths":                         stage.paths,
		"correlation":                   stage.correlation,
		"p_value":                       stage.pValue,
		"sign_agreement":                stage.signAgree,
		"mean_realized_minus_predicted": stage.meanGap,
		"breach_count":                  len(stage.violations),
		"gate":                          gateData(stage.gate),
	}
}
