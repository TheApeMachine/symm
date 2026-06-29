package logic

import (
	"cmp"
	"errors"
	"strings"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/statutil"
)

/*
errUnknownMeasurement is returned by resolve when the evidence a condition needs
is absent from the batch — the signal did not measure this symbol on this tick.
It is NOT a comparison value: a guard like "toxicity is false" must fail closed
when toxicity is absent, never read absence as "false". Conditions catch this
sentinel and treat the condition as unmet rather than fabricating a -1.
*/
var errUnknownMeasurement = errors.New("logic: measurement unknown")

/*
HoldingRef selects flat vs open inventory for one symbol.
*/
type HoldingRef struct {
	Held bool `yaml:"held" json:"held"`
}

/*
ConfidenceRef names a cross-section confidence gate derived from live measurements.
*/
type ConfidenceRef string

const (
	ConfidenceEntryBaseline ConfidenceRef = "entry_baseline"
	ConfidenceExitBaseline  ConfidenceRef = "exit_baseline"
)

/*
ConditionOperand is one side of a playbook comparison.
Fields on the operand replace the removed subject wrapper.
*/
type ConditionOperand struct {
	Source     SourceType    `yaml:"source,omitempty" json:"source,omitempty"`
	Type       SubjectType   `yaml:"type" json:"type"`
	Category   *Category     `yaml:"category,omitempty" json:"category,omitempty"`
	Holding    *HoldingRef   `yaml:"holding,omitempty" json:"holding,omitempty"`
	Confidence ConfidenceRef `yaml:"confidence,omitempty" json:"confidence,omitempty"`
	Eigenmode  *EigenmodeRef `yaml:"eigenmode,omitempty" json:"eigenmode,omitempty"`
	ModeShare  float64       `yaml:"mode_share,omitempty" json:"mode_share,omitempty"`
}

func (operand *ConditionOperand) Compare(
	measurements []*datura.Artifact,
	holdings *datura.Artifact,
	other ConditionOperand,
) (int, error) {
	left, err := operand.resolve(measurements, holdings)

	if err != nil {
		return 0, err
	}

	right, err := other.resolve(measurements, holdings)

	if err != nil {
		return 0, err
	}

	return cmp.Compare(left, right), nil
}

func (operand *ConditionOperand) resolve(
	measurements []*datura.Artifact,
	holdings *datura.Artifact,
) (float64, error) {
	switch operand.Type {
	case SubjectHolding:
		if operand.Holding == nil {
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: holding operand missing held flag",
				nil,
			))
		}

		symbol := symbolFromMeasurements(measurements)
		asset, _, _ := strings.Cut(symbol, "/")
		asset = strings.ToUpper(strings.TrimSpace(asset))
		data := datura.Peek[[]any](holdings, "data")

		if len(data) == 0 {
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: balances artifact missing data",
				nil,
			))
		}

		held := false
		for index := range data {
			if strings.EqualFold(datura.Peek[string](holdings, "data", index, "asset"), asset) {
				held = datura.Peek[float64](holdings, "data", index, "balance") > 0
				break
			}
		}

		if operand.Holding.Held == held {
			return 1, nil
		}

		return -1, nil
	case SubjectCategory:
		if operand.Category == nil {
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: category operand missing category type",
				nil,
			))
		}

		measurement, ok := measurementForSource(measurements, operand.Source)

		if !ok {
			// The signal did not measure this symbol this tick. Absence is
			// unknown, not "category not present": a guard must not pass on
			// missing evidence. Fail closed.
			return 0, errUnknownMeasurement
		}

		index := int(datura.Peek[float64](measurement, "output", "value"))
		if index == 0 {
			index = int(datura.Peek[float64](measurement, "output", "category"))
		}

		confidence := datura.Peek[float64](measurement, "output", "confidence")
		if confidence <= 0 {
			return -1, nil
		}

		categoryType, ok := Categories[index]

		if ok && categoryType == operand.Category.Type {
			return confidence, nil
		}

		return -1, nil
	case SubjectConfidence:
		if operand.Confidence != "" {
			return confidenceBaseline(measurements, operand.Confidence)
		}

		measurement, ok := measurementForSource(measurements, operand.Source)

		if !ok {
			return 0, errUnknownMeasurement
		}

		return datura.Peek[float64](measurement, "output", "confidence"), nil
	case SubjectStrength:
		measurement, ok := measurementForSource(measurements, operand.Source)

		if !ok {
			return 0, errUnknownMeasurement
		}

		return datura.Peek[float64](measurement, "output", "strength"), nil
	case SubjectSurprise:
		measurement, ok := measurementForSource(measurements, operand.Source)

		if !ok {
			return 0, errUnknownMeasurement
		}

		return datura.Peek[float64](measurement, "output", "surprise"), nil
	case SubjectElapsed:
		measurement, ok := measurementForSource(measurements, operand.Source)

		if !ok {
			return 0, errUnknownMeasurement
		}

		// elapsed is the cadence-derived intervals since the prior category on
		// this origin (or a cross-origin anchor), published by the signal on the
		// measurement artifact. It is interval-count, not fixed seconds — the
		// playbook compares it to derived bounds, never to a hardcoded window.
		return datura.Peek[float64](measurement, "output", "elapsed"), nil
	case SubjectEigenmode:
		if operand.Eigenmode == nil {
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: eigenmode operand missing mode",
				nil,
			))
		}

		if operand.Eigenmode.Baseline {
			baseline, ok := EigenmodeShareBaseline(measurements)

			if !ok {
				return 0, errUnknownMeasurement
			}

			return baseline, nil
		}

		score, ok := EigenmodeScore(measurements, operand.Eigenmode.Mode)

		if !ok {
			return 0, errUnknownMeasurement
		}

		return score, nil
	case SubjectModeShare:
		return operand.ModeShare, nil
	default:
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"logic: unsupported operand type: "+string(operand.Type),
			nil,
		))
	}
}

func symbolFromMeasurements(measurements []*datura.Artifact) string {
	if len(measurements) == 0 {
		return ""
	}

	scope, _ := measurements[0].Scope()

	return scope
}

func measurementForSource(
	measurements []*datura.Artifact,
	source SourceType,
) (*datura.Artifact, bool) {
	for _, measurement := range measurements {
		origin, err := measurement.Origin()

		if err != nil {
			continue
		}

		if source != SourceNone && SourceType(origin) != source {
			continue
		}

		return measurement, true
	}

	return nil, false
}

func confidenceBaseline(
	measurements []*datura.Artifact,
	reference ConfidenceRef,
) (float64, error) {
	confidences := make([]float64, 0, len(measurements))

	for _, measurement := range measurements {
		confidence := datura.Peek[float64](measurement, "output", "confidence")

		if confidence <= 0 {
			continue
		}

		confidences = append(confidences, confidence)
	}

	if len(confidences) == 0 {
		return 0, nil
	}

	median := statutil.Median(confidences)
	mad := statutil.MedianAbsoluteDeviation(confidences, median)

	lower, upper, err := statutil.Quartiles(confidences)

	if err != nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"logic: quartiles failed",
			err,
		))
	}

	span := upper - lower
	useMAD := len(confidences) < 4 || span <= mad

	switch reference {
	case ConfidenceEntryBaseline:
		if useMAD {
			return median + mad, nil
		}

		return upper, nil
	case ConfidenceExitBaseline:
		if useMAD {
			return median - mad, nil
		}

		return lower, nil
	default:
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"logic: unknown confidence baseline: "+string(reference),
			nil,
		))
	}
}
