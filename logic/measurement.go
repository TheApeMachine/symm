package logic

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
)

const measurementDistributionTolerance = 1e-6

type SourceType string

const (
	SourceNone        SourceType = ""
	SourceFluid       SourceType = "fluid"
	SourceHawkes      SourceType = "hawkes"
	SourcePumpDump    SourceType = "pumpdump"
	SourceDepthFlow   SourceType = "depthflow"
	SourceSentiment   SourceType = "sentiment"
	SourceCorrelation SourceType = "correlation"
	SourceCausal      SourceType = "causal"
	SourceLeadLag     SourceType = "leadlag"
	SourceLiquidity   SourceType = "liquidity"
	SourceExhaustion  SourceType = "exhaustion"
	SourcePrediction  SourceType = "prediction"
	SourceCVD         SourceType = "cvd"
	SourceToxicity    SourceType = "toxicity"
	SourceManifold    SourceType = "manifold"
	SourceResonance   SourceType = "resonance"
	SourceRegime      SourceType = "regime"
)

/*
Measurement is one signal's category distribution for one symbol.
It is not a transport envelope.
*/
type Measurement struct {
	Source              SourceType
	Symbol              string
	At                  time.Time
	Distribution        map[CategoryType]float64
	Confidence          float64
	Strength            float64
	EntryBaseline       float64
	ExitBaseline        float64
	Surprise            float64
	Elapsed             float64
	CounterfactualReady bool
	Metrics             map[string]float64
	Status              string
	Eigenmode           Eigenmode
	Story               StoryTrace
}

type Eigenmode struct {
	Labels    []string
	Origins   []float64
	Energies  []float64
	Coupling  []float64
	Threshold float64
}

type StoryTrace struct {
	Status           string
	Symbol           string
	Source           SourceType
	Category         CategoryType
	Terminal         string
	TerminalBranchID string
	Evaluated        bool
	Candidates       int
}

type BalanceRow struct {
	Asset   string
	Balance float64
}

type Holdings struct {
	Rows []BalanceRow
}

/*
NewMeasurement creates an empty category distribution for one source and symbol.
*/
func NewMeasurement(source SourceType, symbol string, at time.Time) *Measurement {
	return &Measurement{
		Source:       source,
		Symbol:       symbol,
		At:           at,
		Distribution: map[CategoryType]float64{},
		Metrics:      map[string]float64{},
	}
}

/*
AddMetric records one scalar output on the measurement.
*/
func (measurement *Measurement) AddMetric(name string, value float64) {
	if strings.TrimSpace(name) == "" || value == 0 {
		return
	}

	measurement.Metrics[name] = value
}

/*
ApplyClassifier records the numeric classifier result on the measurement.
*/
func (measurement *Measurement) ApplyClassifier(
	value float64,
	confidence float64,
	entryBaseline float64,
	exitBaseline float64,
	strength float64,
	distribution map[string]float64,
) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return errnie.Err(errnie.Validation, "logic: measurement value is non-finite", nil)
	}

	if !finiteConfidence(confidence) {
		return errnie.Err(errnie.Validation, "logic: measurement confidence must be finite and between zero and one", nil)
	}

	if !finiteBaseline(entryBaseline) {
		return errnie.Err(errnie.Validation, "logic: measurement entry baseline must be finite and between zero and one", nil)
	}

	if !finiteBaseline(exitBaseline) {
		return errnie.Err(errnie.Validation, "logic: measurement exit baseline must be finite and between zero and one", nil)
	}

	if strength < 0 || math.IsNaN(strength) || math.IsInf(strength, 0) {
		return errnie.Err(errnie.Validation, "logic: measurement strength must be finite", nil)
	}

	measurement.Confidence = confidence
	measurement.EntryBaseline = entryBaseline
	measurement.ExitBaseline = exitBaseline
	measurement.Strength = strength
	measurement.AddMetric("value", value)
	total := 0.0

	for key, probability := range distribution {
		category, err := measurement.category(key)
		if err != nil {
			return err
		}

		if math.IsNaN(probability) || math.IsInf(probability, 0) || probability < 0 {
			return errnie.Err(errnie.Validation, "logic: measurement distribution is non-finite", nil)
		}

		if category == CategoryTypeNone || probability <= 0 {
			continue
		}

		measurement.Distribution[category] = probability
		total += probability
	}

	if math.Abs(total-1) > measurementDistributionTolerance {
		return errnie.Err(errnie.Validation, "logic: measurement distribution must sum to one", nil)
	}

	return nil
}

/*
category reads either a category index or category name.
*/
func (measurement *Measurement) category(key string) (CategoryType, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return CategoryTypeNone, nil
	}

	index, err := strconv.Atoi(key)
	if err == nil {
		category := Categories[index]
		if category == CategoryTypeNone {
			return CategoryTypeNone, errnie.Err(
				errnie.Validation,
				"logic: unknown category index "+key,
				nil,
			)
		}

		return category, nil
	}

	category := CategoryType(key)
	if !KnownCategory(category) {
		return CategoryTypeNone, errnie.Err(
			errnie.Validation,
			"logic: unknown category "+key,
			nil,
		)
	}

	return category, nil
}

func (measurement *Measurement) Ready() error {
	if measurement == nil {
		return errnie.Err(errnie.Validation, "logic: nil measurement", nil)
	}

	if strings.TrimSpace(measurement.Symbol) == "" {
		return errnie.Err(errnie.Validation, "logic: measurement symbol required", nil)
	}

	if measurement.Source == SourceNone {
		return errnie.Err(errnie.Validation, "logic: measurement source required", nil)
	}

	if !measurement.HasDistribution() {
		return errnie.Err(errnie.Validation, "logic: measurement distribution required", nil)
	}

	if !finiteConfidence(measurement.Confidence) {
		return errnie.Err(errnie.Validation, "logic: measurement confidence must be finite and between zero and one", nil)
	}

	if !finiteBaseline(measurement.EntryBaseline) {
		return errnie.Err(errnie.Validation, "logic: measurement entry baseline must be finite and between zero and one", nil)
	}

	if !finiteBaseline(measurement.ExitBaseline) {
		return errnie.Err(errnie.Validation, "logic: measurement exit baseline must be finite and between zero and one", nil)
	}

	if measurement.Strength < 0 ||
		math.IsNaN(measurement.Strength) ||
		math.IsInf(measurement.Strength, 0) {
		return errnie.Err(errnie.Validation, "logic: measurement strength must be finite", nil)
	}

	total := 0.0

	for category, mass := range measurement.Distribution {
		if !KnownCategory(category) {
			return errnie.Err(errnie.Validation, "logic: measurement distribution category unknown", nil)
		}

		if mass < 0 || math.IsNaN(mass) || math.IsInf(mass, 0) {
			return errnie.Err(errnie.Validation, "logic: measurement distribution mass must be finite", nil)
		}

		total += mass
	}

	if math.Abs(total-1) > measurementDistributionTolerance {
		return errnie.Err(errnie.Validation, "logic: measurement distribution must sum to one", nil)
	}

	return nil
}

func (measurement *Measurement) HasDistribution() bool {
	for category, mass := range measurement.Distribution {
		if category != CategoryTypeNone && mass > 0 {
			return true
		}
	}

	return false
}

func (measurement *Measurement) CategoryMass(category CategoryType) float64 {
	if measurement == nil || category == CategoryTypeNone {
		return 0
	}

	return measurement.Distribution[category]
}

func (measurement *Measurement) DominantCategory() CategoryType {
	best := CategoryTypeNone
	bestMass := 0.0

	for category, mass := range measurement.Distribution {
		if category == CategoryTypeNone || mass <= bestMass {
			continue
		}

		best = category
		bestMass = mass
	}

	return best
}

func (measurement *Measurement) Metric(name string) float64 {
	switch strings.TrimSpace(name) {
	case "":
		return measurement.Confidence
	case "confidence":
		return measurement.Confidence
	case "strength":
		return measurement.Strength
	case "surprise":
		return measurement.Surprise
	case "elapsed":
		return measurement.Elapsed
	case "entry_baseline":
		return measurement.EntryBaseline
	case "exit_baseline":
		return measurement.ExitBaseline
	default:
		return measurement.Metrics[name]
	}
}

func (measurement *Measurement) EntryScore() (float64, error) {
	if measurement == nil {
		return 0, errnie.Err(errnie.Validation, "logic: nil measurement", nil)
	}

	if measurement.EntryBaseline <= 0 || measurement.EntryBaseline >= 1 {
		return 0, errnie.Err(
			errnie.Validation,
			"logic: measurement entry baseline must be between zero and one",
			nil,
		)
	}

	score := (measurement.Confidence - measurement.EntryBaseline) /
		(1 - measurement.EntryBaseline)

	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 0, errnie.Err(errnie.Validation, "logic: non-finite entry score", nil)
	}

	return score, nil
}

func finiteConfidence(value float64) bool {
	return value > 0 && value <= 1 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func finiteBaseline(value float64) bool {
	return value > 0 && value < 1 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (holdings *Holdings) Held(asset string) (bool, error) {
	if holdings == nil {
		return false, errnie.Err(errnie.Validation, "logic: holdings required", nil)
	}

	asset = strings.ToUpper(strings.TrimSpace(asset))
	if asset == "" {
		return false, errnie.Err(errnie.Validation, "logic: holding asset required", nil)
	}

	if len(holdings.Rows) == 0 {
		return false, errnie.Err(errnie.Validation, "logic: holdings rows required", nil)
	}

	for _, row := range holdings.Rows {
		if strings.ToUpper(strings.TrimSpace(row.Asset)) == asset {
			return row.Balance > 0, nil
		}
	}

	return false, nil
}
