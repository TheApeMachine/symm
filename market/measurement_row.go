package market

import (
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

func GetMeasurement(row *qpool.QValue[any]) logic.Measurement {
	measurement, ok := row.Value.(logic.Measurement)

	if !ok {
		return logic.Measurement{}
	}

	return measurement
}
