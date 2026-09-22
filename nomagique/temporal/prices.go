package temporal

import (
	"encoding/json"

	"github.com/theapemachine/errnie"
)

type priceReading struct {
	symbol string
	value  float64
}

/* readPrices validates all records before the frame advances any mining state. */
func readPrices(records []map[string]json.RawMessage, field string) ([]priceReading, error) {
	if len(records) == 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "mine: selected channel has no records", nil))
	}
	readings := make([]priceReading, len(records))

	for index, record := range records {
		if err := json.Unmarshal(record["symbol"], &readings[index].symbol); err != nil {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "mine: record symbol", err))
		}

		if err := json.Unmarshal(record[field], &readings[index].value); err != nil {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "mine: record price field "+field, err))
		}

		if readings[index].symbol == "" || readings[index].value <= 0 {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "mine: record requires symbol and positive price", nil))
		}
	}
	return readings, nil
}
