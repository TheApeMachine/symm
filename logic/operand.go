package logic

import (
	"cmp"
	"fmt"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/statutil"
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
	measurements []*datura.Artifact,
	holdings *Balances,
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
	holdings *Balances,
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
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: missing measurement for source "+string(operand.Source),
				nil,
			))
		}

		categoryIndex := CategoryIndex(operand.Category.Type)

		if operand.Category.Type != CategoryTypeNone {
			mass := datura.Peek[float64](
				measurement,
				"output",
				fmt.Sprintf("category.%d", categoryIndex),
			)

			if mass > 0 {
				return mass, nil
			}
		}

		index := int(datura.Peek[float64](measurement, "output", "value"))
		categoryType, ok := Categories[index]

		if ok && categoryType == operand.Category.Type {
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

		return datura.Peek[float64](measurement, "output", "confidence"), nil
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
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: eigenmode score unavailable",
				nil,
			))
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

func symbolHeld(holdings *Balances, symbol string) bool {
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

	lower, upper, err := statutil.Quartiles(confidences)

	if err != nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"logic: quartiles failed",
			err,
		))
	}

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
