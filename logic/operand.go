package logic

import (
	"cmp"
	"math"
	"strings"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/geometry"
)

/*
ConditionOperand is one side of a playbook comparison.
Fields on the operand replace the removed subject wrapper.
*/
type ConditionOperand struct {
	Source     SourceType       `yaml:"source,omitempty" json:"source,omitempty"`
	Type       SubjectType      `yaml:"type" json:"type"`
	Category   *Category        `yaml:"category,omitempty" json:"category,omitempty"`
	Holding    datura.Map[bool] `yaml:"holding,omitempty" json:"holding,omitempty"`
	Confidence string           `yaml:"confidence,omitempty" json:"confidence,omitempty"`
	Eigenmode  datura.Map[any]  `yaml:"eigenmode,omitempty" json:"eigenmode,omitempty"`
}

func (operand *ConditionOperand) Compare(
	measurements []*datura.Artifact,
	holdings *datura.Artifact,
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
	measurements []*datura.Artifact,
	holdings *datura.Artifact,
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

		if holdings == nil {
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: balances artifact required",
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

			symbol = datura.Peek[string](measurements[index], "scope")

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

		rows := datura.Peek[[]any](holdings, "data")

		if len(rows) == 0 {
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: balances data required",
				nil,
			))
		}

		held := false

		for index := range rows {
			asset := datura.Peek[string](holdings, "data", index, "asset")
			balance := datura.Peek[float64](holdings, "data", index, "balance")

			if asset == "" {
				return 0, errnie.Error(errnie.Err(
					errnie.Validation,
					"logic: balances asset required",
					nil,
				).With(holdings.Log()...))
			}

			if asset == base {
				held = balance > 0
				break
			}
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

		index := int(datura.Peek[float64](measurement, "output", "value"))
		categoryType, ok := Categories[index]

		if !ok {
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: unknown measurement category",
				nil,
			).With(measurement.Log()...))
		}

		if ok && categoryType == operand.Category.Type {
			confidence := datura.Peek[float64](measurement, "output", "confidence")
			if confidence <= 0 {
				return 0, errnie.Error(errnie.Err(
					errnie.Validation,
					"logic: measurement confidence required",
					nil,
				).With(measurement.Log()...))
			}

			return confidence, nil
		}

		return -1, nil
	case SubjectConfidence:
		measurement, err := operand.Measurement(measurements)
		if err != nil {
			return 0, err
		}

		if operand.Confidence != "" {
			value := datura.Peek[float64](measurement, "output", operand.Confidence)
			if value <= 0 {
				return 0, errnie.Error(errnie.Err(
					errnie.Validation,
					"logic: measurement output."+operand.Confidence+" required",
					nil,
				).With(measurement.Log()...))
			}

			return value, nil
		}

		value := datura.Peek[float64](measurement, "output", "confidence")
		if value <= 0 {
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: measurement output.confidence required",
				nil,
			).With(measurement.Log()...))
		}

		return value, nil
	case SubjectStrength:
		measurement, err := operand.Measurement(measurements)
		if err != nil {
			return 0, err
		}

		value := datura.Peek[float64](measurement, "output", "strength")
		if value <= 0 {
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: measurement output.strength required",
				nil,
			).With(measurement.Log()...))
		}

		return value, nil
	case SubjectSurprise:
		measurement, err := operand.Measurement(measurements)
		if err != nil {
			return 0, err
		}

		return datura.Peek[float64](measurement, "output", "surprise"), nil
	case SubjectElapsed:
		measurement, err := operand.Measurement(measurements)
		if err != nil {
			return 0, err
		}

		return datura.Peek[float64](measurement, "output", "elapsed"), nil
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

			labels := datura.Peek[[]string](measurement, "output", "eigenmode", "labels")
			origins := datura.Peek[[]float64](measurement, "output", "eigenmode", "origins")
			energies := datura.Peek[[]float64](measurement, "output", "eigenmode", "energies")
			coupling := datura.Peek[[]float64](measurement, "output", "eigenmode", "coupling")

			if len(labels) == 0 && len(origins) == 0 && len(energies) == 0 && len(coupling) == 0 {
				continue
			}

			threshold := datura.Peek[float64](measurement, "output", "eigenmode", "threshold")
			if threshold <= 0 {
				return 0, errnie.Error(errnie.Err(
					errnie.Validation,
					"logic: eigenmode threshold required",
					nil,
				).With(measurement.Log()...))
			}

			if len(labels) != len(origins) || len(origins) != len(energies) || len(coupling) != len(origins)*len(origins) {
				return 0, errnie.Error(errnie.Err(
					errnie.Validation,
					"logic: eigenmode labels, origins, energies, and coupling must align",
					nil,
				).With(measurement.Log()...))
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
					).With(measurement.Log()...))
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
				return 0, errnie.Error(errnie.Err(
					errnie.Validation,
					"logic: eigenmode label not found: "+modeName,
					nil,
				).With(measurement.Log()...))
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
				return 0, errnie.Error(errnie.Err(
					errnie.Validation,
					"logic: eigenmode partition empty",
					nil,
				).With(measurement.Log()...))
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
				).With(measurement.Log()...))
			}

			for modeIndex := range modes {
				members := modes[modeIndex].Members()

				for memberIndex := range members {
					if members[memberIndex] == targetOrigin {
						return modes[modeIndex].Energy() / total, nil
					}
				}
			}

			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: eigenmode member not found: "+modeName,
				nil,
			).With(measurement.Log()...))
		}

		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"logic: eigenmode measurement required",
			nil,
		))
	default:
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"logic: unsupported operand type: "+string(operand.Type),
			nil,
		))
	}
}

func (operand *ConditionOperand) Measurement(
	measurements []*datura.Artifact,
) (*datura.Artifact, error) {
	var newest *datura.Artifact
	var newestStamp int64

	for _, measurement := range measurements {
		if measurement == nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: nil measurement",
				nil,
			))
		}

		origin := datura.Peek[string](measurement, "origin")

		if operand.Source != SourceNone && SourceType(origin) != operand.Source {
			continue
		}

		if origin == "" {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"logic: measurement origin required",
				nil,
			).With(measurement.Log()...))
		}

		stamp := measurement.Timestamp()
		if newest == nil || stamp >= newestStamp {
			newest = measurement
			newestStamp = stamp
		}
	}

	if newest == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"logic: measurement source required: "+string(operand.Source),
			nil,
		))
	}

	return newest, nil
}
