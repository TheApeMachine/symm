package logic

import (
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

type clampProjector struct {
	schemas map[types.SourceType][]metricSchema
}

type clampSample struct {
	rho      float64
	momX     float64
	momY     float64
	momZ     float64
	energy   float64
	pressure float64
}

func newClampProjector() *clampProjector {
	return &clampProjector{schemas: sourceMetricSchemas()}
}

func (projector *clampProjector) Project(
	source types.SourceType,
	measurement *types.Measurement,
) (clampSample, error) {
	if err := projector.validate(source, measurement); err != nil {
		return clampSample{}, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("decision boundary: %s clamp failed to project", source),
			err,
		))
	}

	distribution, err := newCategoryDistribution(measurement.Categories)
	if err != nil {
		return clampSample{}, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("decision boundary: %s clamp failed to project", source),
			err,
		))
	}

	return distribution.sample(), nil
}

func (projector *clampProjector) validate(
	source types.SourceType,
	measurement *types.Measurement,
) error {
	schemas := projector.schemas[source]
	if len(schemas) == 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("decision boundary: unknown source %q", source),
			nil,
		))
	}

	for _, schema := range schemas {
		if schema.Matches(measurement) {
			return nil
		}
	}

	if len(schemas) == 1 {
		return schemas[0].Error(measurement)
	}

	return errnie.Error(errnie.Err(
		errnie.Validation,
		fmt.Sprintf(
			"decision boundary: %s measurement does not match any known metric schema",
			source,
		),
		nil,
	))
}

type categoryDistribution struct {
	rows []weightedCategory
}

type weightedCategory struct {
	category types.Category
	role     categoryRole
	weight   float64
}

func newCategoryDistribution(
	categories []types.Category,
) (categoryDistribution, error) {
	if len(categories) == 0 {
		return categoryDistribution{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision boundary: categories required",
			nil,
		))
	}

	totalStrength := 0.0
	for _, category := range categories {
		totalStrength += positive(category.Strength)
	}

	if totalStrength <= 0 {
		return categoryDistribution{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision boundary: category strength required",
			nil,
		))
	}

	rows := make([]weightedCategory, 0, len(categories))
	for _, category := range categories {
		role, err := roleForCategory(category.Type)
		if err != nil {
			return categoryDistribution{}, errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("decision boundary: category %q role required", category.Type),
				nil,
			))
		}

		weight := category.Confidence * positive(category.Strength) / totalStrength
		if weight <= 0 {
			continue
		}

		rows = append(rows, weightedCategory{
			category: category,
			role:     role,
			weight:   weight,
		})
	}

	if len(rows) == 0 {
		return categoryDistribution{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision boundary: category confidence required",
			nil,
		))
	}

	return categoryDistribution{rows: rows}, nil
}

func (distribution categoryDistribution) sample() clampSample {
	sample := clampSample{}

	for _, row := range distribution.rows {
		surprisal := positive(row.category.Surprisal)
		sample.rho += row.weight
		sample.momX += row.role.direction * row.weight
		sample.momY += row.role.risk * row.weight
		sample.momZ += row.role.support * row.weight
		sample.energy += row.weight + surprisal*row.weight
	}

	sample.pressure = math.Abs(sample.momX) + sample.momY + sample.momZ
	if sample.pressure <= 0 {
		sample.pressure = sample.rho
	}

	return sample
}

func positive(value float64) float64 {
	if value <= 0 {
		return 0
	}

	return value
}
