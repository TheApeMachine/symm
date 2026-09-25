package data

import (
	"encoding/json"
	"fmt"

	"github.com/theapemachine/errnie"
)

// project is the numeric-port form of MeasurementService. It preserves a
// producer's full declared coordinate set, including explicitly absent values.
// Signals and dependent logic modules use exactly the same publication shape.
func (server *MeasurementServiceServer) project(measurement Measurement, args MeasurementService_write_Params) error {
	run, err := args.Run()

	if err != nil {
		return measurementProjectionError("read run", err)
	}

	producer, err := args.Producer()

	if err != nil {
		return measurementProjectionError("read producer", err)
	}

	declaration, err := args.Coordinates()

	if err != nil {
		return measurementProjectionError("read coordinates", err)
	}

	if run == "" && producer == "" && declaration == "" {
		return nil
	}

	if run == "" || producer == "" || declaration == "" || args.Tick() <= 0 {
		return measurementProjectionError("run, producer, coordinates and positive boundary tick are required", nil)
	}

	var coordinates []uint32

	if err := json.Unmarshal([]byte(declaration), &coordinates); err != nil {
		return measurementProjectionError("invalid coordinate declaration", err)
	}

	if len(coordinates) == 0 {
		return measurementProjectionError("empty coordinate declaration", nil)
	}

	if err := measurement.SetRun(run); err != nil {
		return measurementProjectionError("set run", err)
	}

	if err := measurement.SetProducer(producer); err != nil {
		return measurementProjectionError("set producer", err)
	}

	values, err := args.Values()

	if err != nil {
		return measurementProjectionError("read values", err)
	}

	present, err := args.Present()

	if err != nil {
		return measurementProjectionError("read presence", err)
	}

	metrics, err := args.Metrics()

	if err != nil {
		return measurementProjectionError("read metrics", err)
	}

	if metrics.Len() > 0 && values.Len() > 0 {
		return measurementProjectionError("metrics and numeric ports cannot both own the same publication", nil)
	}

	count := max(metrics.Len(), values.Len())

	if count > 0 && count != len(coordinates) {
		return measurementProjectionError(fmt.Sprintf("%d readings for %d declared coordinates", count, len(coordinates)), nil)
	}

	if present.Len() != count {
		return measurementProjectionError("one explicit presence flag is required per supplied reading", nil)
	}

	addresses, err := measurement.NewCoordinates(int32(len(coordinates)))

	if err != nil {
		return measurementProjectionError("allocate coordinates", err)
	}

	flags, err := measurement.NewPresent(int32(len(coordinates)))

	if err != nil {
		return measurementProjectionError("allocate presence", err)
	}

	if metrics.Len() == 0 {
		metrics, err = measurement.NewMetrics(int32(len(coordinates)))

		if err != nil {
			return measurementProjectionError("allocate metrics", err)
		}
	}

	seen := make(map[uint32]bool, len(coordinates))

	for index, coordinate := range coordinates {
		if seen[coordinate] {
			return measurementProjectionError("duplicate coordinate in publication", nil)
		}

		seen[coordinate] = true
		addresses.Set(index, coordinate)

		if count == 0 {
			continue
		}

		flags.Set(index, present.At(index))

		if values.Len() > 0 {
			metrics.At(index).SetRaw(values.At(index))
		}
	}

	return nil
}

func measurementProjectionError(message string, cause error) error {
	return errnie.Error(errnie.Err(errnie.Validation, "data.measurement: "+message, cause))
}
