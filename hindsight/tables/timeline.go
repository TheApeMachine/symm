package tables

import (
	"context"
	"iter"
	"slices"

	"github.com/apache/iceberg-go"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Timeline reconstructs the sequential market tape from the unified Measurements table.
*/
func (catalog *Catalog) Timeline(
	ctx context.Context,
	epoch int64,
	label string,
	fromTick int64,
	toTick int64,
) iter.Seq[*data.Measurement[float64]] {
	var filter iceberg.BooleanExpression

	if label != "" {
		filter = iceberg.EqualTo(iceberg.Reference("label"), label)
	}

	if fromTick > 0 {
		expression := iceberg.BooleanExpression(iceberg.GreaterThanEqual(iceberg.Reference("seqIdx"), fromTick))

		if filter != nil {
			expression = iceberg.NewAnd(filter, expression)
		}

		filter = expression
	}

	if toTick > 0 && toTick >= fromTick {
		expression := iceberg.BooleanExpression(iceberg.LessThanEqual(iceberg.Reference("seqIdx"), toTick))

		if filter != nil {
			expression = iceberg.NewAnd(filter, expression)
		}

		filter = expression
	}

	return catalog.Scan(ctx, Measurements, epoch, filter, 0)
}

/*
Labels discovers distinct labels present in an epoch by reading only the label column.
*/
func (catalog *Catalog) Labels(ctx context.Context, epoch int64) ([]string, error) {
	labelSet := make(map[string]struct{})

	for measurement := range catalog.Scan(ctx, Measurements, epoch, nil, 0, "label") {
		if measurement.Label != "" {
			labelSet[measurement.Label] = struct{}{}
		}
	}

	labels := make([]string, 0, len(labelSet))

	for label := range labelSet {
		labels = append(labels, label)
	}

	slices.Sort(labels)

	return labels, nil
}
