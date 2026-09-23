package temporal

import (
	"encoding/json"

	"github.com/theapemachine/errnie"
)

type priceReading struct {
	record int
	symbol string
	value  float64
}

/* readPrices validates all records before the frame advances any mining state. */
func readPrices(records []map[string]json.RawMessage, field string) ([]priceReading, error) {
	if len(records) == 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "mine: selected channel has no records", nil))
	}
	readings := make([]priceReading, 0, len(records))

	for index, record := range records {
		reading := priceReading{record: index}

		if err := json.Unmarshal(record["symbol"], &reading.symbol); err != nil {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "mine: record symbol", err))
		}

		if err := json.Unmarshal(record[field], &reading.value); err != nil {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "mine: record price field "+field, err))
		}

		if reading.symbol == "" || reading.value < 0 {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "mine: record requires symbol and non-negative price", nil))
		}

		// A pair that has never traded reports zero: there is no price to mine yet.
		if reading.value == 0 {
			continue
		}
		readings = append(readings, reading)
	}
	return readings, nil
}
