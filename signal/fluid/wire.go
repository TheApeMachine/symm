package fluid

import "math"

/*
WireRow maps one symbol field reading to the dashboard wire shape.
Non-finite floats are rejected because JSON cannot encode them and a bad
field_snapshot frame would tear down the dashboard websocket.
*/
func WireRow(row map[string]any) map[string]any {
	for _, value := range row {
		number, ok := value.(float64)

		if !ok {
			continue
		}

		if math.IsNaN(number) || math.IsInf(number, 0) {
			return nil
		}
	}

	return row
}
