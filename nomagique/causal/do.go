package causal

func doExpectation(
	nodeTable nodeTable,
	treatment int,
	level float64,
	controls ...int,
) (float64, error) {
	predictors := append(append([]int(nil), controls...), treatment)
	model, err := nodeTable.fitLinearModel(predictors...)

	if err != nil {
		return 0, err
	}

	total := 0.0

	for _, row := range nodeTable.rows {
		prediction, err := model.predict(row, treatment, level)

		if err != nil {
			return 0, err
		}

		total += prediction
	}

	return total / float64(len(nodeTable.rows)), nil
}
