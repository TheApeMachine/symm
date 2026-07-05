package logic

import (
	"cmp"
	"math"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/geometry"
)

/*
ConditionOperand is one side of a playbook comparison.
Fields on the operand replace the removed subject wrapper.
*/
type ConditionOperand struct {
	Source     SourceType      `yaml:"source,omitempty" json:"source,omitempty"`
	Type       SubjectType     `yaml:"type" json:"type"`
	Category   *Category       `yaml:"category,omitempty" json:"category,omitempty"`
	Holding    map[string]bool `yaml:"holding,omitempty" json:"holding,omitempty"`
	Confidence string          `yaml:"confidence,omitempty" json:"confidence,omitempty"`
	Eigenmode  map[string]any  `yaml:"eigenmode,omitempty" json:"eigenmode,omitempty"`
}

func (operand *ConditionOperand) Compare(
	measurements []*Measurement,
	holdings *Holdings,
	other ConditionOperand,
) (int, error) {
	if other.Type == SubjectConfidence &&
		other.Source == SourceNone &&
		other.Confidence != "" {
		other.Source = operand.Source
	}

	left, err := operand.Resolve(measurements, holdings)

	if err != nil {
		return 0, err
	}

	right, err := other.Resolve(measurements, holdings)

	if err != nil {
		return 0, err
	}

	return cmp.Compare(left, right), nil
}

func (operand *ConditionOperand) Resolve(
	measurements []*Measurement,
	holdings *Holdings,
) (float64, error) {
	switch operand.Type {
	case SubjectHolding:
		expected, ok := operand.Holding["held"]

		if !ok {
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: holding operand requires held",
				nil,
			))
		}

		symbol := ""

		for index := range measurements {
			if measurements[index] == nil {
				return 0, errnie.Error(errnie.Err(
					errnie.Validation,
					"logic: nil measurement",
					nil,
				))
			}

			symbol = measurements[index].Symbol

			if symbol != "" {
				break
			}
		}

		base, _, ok := strings.Cut(symbol, "/")

		if !ok || base == "" {
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: measurement scope pair required for holding operand",
				nil,
			))
		}

		held, err := holdings.Held(base)
		if err != nil {
			return 0, err
		}

		if held == expected {
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

		measurement, err := operand.Measurement(measurements)
		if err != nil {
			return 0, err
		}
		if measurement == nil {
			return 0, nil
		}

		mass := measurement.CategoryMass(operand.Category.Type)
		if mass > 0 {
			return mass, nil
		}

		return -1, nil
	case SubjectConfidence:
		measurement, err := operand.Measurement(measurements)
		if err != nil {
			return 0, err
		}
		if measurement == nil {
			return 0, nil
		}

		if operand.Confidence != "" {
			value := measurement.Metric(operand.Confidence)
			if value <= 0 {
				return 0, errnie.Error(errnie.Err(
					errnie.Validation,
					"logic: measurement "+operand.Confidence+" required",
					nil,
				))
			}

			return value, nil
		}

		value := measurement.Confidence
		if value <= 0 {
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: measurement confidence required",
				nil,
			))
		}

		return value, nil
	case SubjectStrength:
		measurement, err := operand.Measurement(measurements)
		if err != nil {
			return 0, err
		}
		if measurement == nil {
			return 0, nil
		}

		value := measurement.Strength
		if value <= 0 {
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: measurement strength required",
				nil,
			))
		}

		return value, nil
	case SubjectSurprise:
		measurement, err := operand.Measurement(measurements)
		if err != nil {
			return 0, err
		}
		if measurement == nil {
			return 0, nil
		}

		return measurement.Surprise, nil
	case SubjectElapsed:
		measurement, err := operand.Measurement(measurements)
		if err != nil {
			return 0, err
		}
		if measurement == nil {
			return 0, nil
		}

		return measurement.Elapsed, nil
	case SubjectEigenmode:
		modeName, ok := operand.Eigenmode["mode"].(string)
		if !ok || modeName == "" {
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: eigenmode operand missing mode",
				nil,
			))
		}

		baseline, _ := operand.Eigenmode["baseline"].(bool)

		for index := range measurements {
			measurement := measurements[index]
			if measurement == nil {
				return 0, errnie.Error(errnie.Err(
					errnie.Validation,
					"logic: nil measurement",
					nil,
				))
			}

			labels := measurement.Eigenmode.Labels
			origins := measurement.Eigenmode.Origins
			energies := measurement.Eigenmode.Energies
			coupling := measurement.Eigenmode.Coupling

			if len(labels) == 0 && len(origins) == 0 && len(energies) == 0 && len(coupling) == 0 {
				continue
			}

			threshold := measurement.Eigenmode.Threshold
			if threshold <= 0 {
				return 0, errnie.Error(errnie.Err(
					errnie.Validation,
					"logic: eigenmode threshold required",
					nil,
				))
			}

			if len(labels) != len(origins) || len(origins) != len(energies) || len(coupling) != len(origins)*len(origins) {
				return 0, errnie.Error(errnie.Err(
					errnie.Validation,
					"logic: eigenmode labels, origins, energies, and coupling must align",
					nil,
				))
			}

			participants := make([]geometry.ModeParticipant, len(origins))
			targetOrigin := uint64(0)
			targetFound := false

			for participantIndex := range origins {
				if origins[participantIndex] < 0 || math.Trunc(origins[participantIndex]) != origins[participantIndex] {
					return 0, errnie.Error(errnie.Err(
						errnie.Validation,
						"logic: eigenmode origin must be unsigned integer",
						nil,
					))
				}

				origin := uint64(origins[participantIndex])
				participants[participantIndex] = geometry.ModeParticipant{
					Origin: origin,
					Energy: energies[participantIndex],
				}

				if labels[participantIndex] == modeName {
					targetOrigin = origin
					targetFound = true
				}
			}

			if !targetFound {
				return 0, nil
			}

			couplingFn := func(leftOrigin, rightOrigin uint64) float64 {
				left := -1
				right := -1

				for originIndex := range origins {
					origin := uint64(origins[originIndex])

					if origin == leftOrigin {
						left = originIndex
					}

					if origin == rightOrigin {
						right = originIndex
					}
				}

				if left < 0 || right < 0 {
					return 0
				}

				return coupling[left*len(origins)+right]
			}

			modes, dominant := geometry.DetectModes(participants, threshold, couplingFn)
			if dominant < 0 || len(modes) == 0 {
				return 0, nil
			}

			if baseline {
				return 1 / float64(len(modes)), nil
			}

			total := 0.0
			for modeIndex := range modes {
				total += modes[modeIndex].Energy()
			}

			if total <= 0 {
				return 0, errnie.Error(errnie.Err(
					errnie.Validation,
					"logic: eigenmode energy required",
					nil,
				))
			}

			for modeIndex := range modes {
				members := modes[modeIndex].Members()

				for memberIndex := range members {
					if members[memberIndex] == targetOrigin {
						return modes[modeIndex].Energy() / total, nil
					}
				}
			}

			return 0, nil
		}

		return 0, nil
	default:
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"logic: unsupported operand type: "+string(operand.Type),
			nil,
		))
	}
}

func (operand *ConditionOperand) Measurement(
	measurements []*Measurement,
) (*Measurement, error) {
	var newest *Measurement
	var newestStamp int64

	for _, measurement := range measurements {
		if measurement == nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: nil measurement",
				nil,
			))
		}

		if operand.Source != SourceNone && measurement.Source != operand.Source {
			continue
		}

		if measurement.Source == SourceNone {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: measurement source required",
				nil,
			))
		}

		stamp := measurement.At.UnixNano()
		if newest == nil || stamp >= newestStamp {
			newest = measurement
			newestStamp = stamp
		}
	}

	return newest, nil
}
