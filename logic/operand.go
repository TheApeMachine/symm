package logic

import (
	"cmp"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken/user"
)

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
	measurements []Measurement,
	holdings *user.Balances,
	other ConditionOperand,
) (int, error) {
	left, leftErr := operand.resolve(measurements, holdings)

	if leftErr != nil {
		return 0, leftErr
	}

	right, rightErr := other.resolve(measurements, holdings)

	if rightErr != nil {
		return 0, rightErr
	}

	return cmp.Compare(left, right), nil
}

func (operand *ConditionOperand) resolve(
	measurements []Measurement,
	holdings *user.Balances,
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

		held := symbolHeld(holdings, symbolFromMeasurements(measurements))

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
			return -1, nil
		}

		if measurement.Category == operand.Category.Type {
			return 1, nil
		}

		return -1, nil
	case SubjectConfidence:
		if operand.Confidence != "" {
			return confidenceBaseline(measurements, operand.Confidence)
		}

		measurement, ok := measurementForSource(measurements, operand.Source)

		if !ok {
			return -1, nil
		}

		return measurement.Confidence, nil
	case SubjectEigenmode:
		if operand.Eigenmode == nil {
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: eigenmode operand missing mode",
				nil,
			))
		}

		score, ok := EigenmodeScore(measurements, operand.Eigenmode.Mode)

		if !ok {
			return 0, nil
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

func symbolFromMeasurements(measurements []Measurement) string {
	if len(measurements) == 0 {
		return ""
	}

	return measurements[0].Symbol
}

func measurementForSource(
	measurements []Measurement,
	source SourceType,
) (Measurement, bool) {
	for _, measurement := range measurements {
		if source != SourceNone && measurement.Source != source {
			continue
		}

		return measurement, true
	}

	return Measurement{}, false
}

func symbolHeld(holdings *user.Balances, symbol string) bool {
	if holdings == nil || symbol == "" {
		return false
	}

	if quantity, ok := holdings.Inventory[symbol]; ok && quantity > 0 {
		return true
	}

	for _, asset := range holdings.Asset {
		if asset.Asset == symbol && asset.Balance > 0 {
			return true
		}
	}

	return false
}

func confidenceBaseline(
	measurements []Measurement,
	reference ConfidenceRef,
) (float64, error) {
	confidences := make([]float64, 0, len(measurements))

	for _, measurement := range measurements {
		if measurement.Confidence <= 0 {
			continue
		}

		confidences = append(confidences, measurement.Confidence)
	}

	if len(confidences) == 0 {
		return 1, nil
	}

	lower, upper := statistic.Quartiles(confidences)

	switch reference {
	case ConfidenceEntryBaseline:
		return upper, nil
	case ConfidenceExitBaseline:
		return lower, nil
	default:
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"logic: unknown confidence baseline: "+string(reference),
			nil,
		))
	}
}
