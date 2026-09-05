package cmd

import (
	"reflect"
	"strconv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

/*
project reuses three Measurement objects per key for scalar logic readouts.
Scalars are reflected from the declared readout structs, so adding a numeric
field cannot silently omit it. Resident tensor/particle state is not copied:
the solvers' scalar readouts and numerical inference vectors are the outputs.
*/
func (node *gridNode) project(envelope *types.Envelope, output []*data.Measurement[float64]) error {
	key := envelopeSymbol(envelope)

	if key == "" {
		return nil
	}

	if node.projections == nil {
		node.projections = make(map[string]*[3]data.Measurement[float64])
		node.vectors = make(map[string][]string)
	}

	projections := node.projections[key]

	if projections == nil {
		projections = &[3]data.Measurement[float64]{}
		for index, source := range []string{"cognition", "resonance", "manifold"} {
			projections[index] = *data.NewMeasurement[float64]("", key, source, processStartedAt, processStartedAt)
		}
		node.projections[key] = projections
	}

	if reading := envelope.Cognition; reading != nil {
		if reading.Error != "" {
			return errnie.Err(errnie.Validation, "grid: cognition output: "+reading.Error, nil)
		}
		measurement := &projections[0]
		clear(measurement.Metrics)
		measurement.At = reading.At
		node.scalars(measurement, "", reading)
		for _, class := range reading.Classes {
			measurement.PutMetric(data.Metric[float64]{Label: "class." + class.Name, Raw: class.Probability})
		}
		for label, value := range reading.Predictions {
			measurement.PutMetric(data.Metric[float64]{Label: "prediction." + label, Raw: value})
		}
		output[0] = measurement
	}

	if reading := envelope.Resonance; reading != nil {
		measurement := &projections[1]
		clear(measurement.Metrics)
		measurement.At = reading.At
		node.scalars(measurement, "", reading)
		node.scalars(measurement, "dynamics.", reading.Dynamics)
		node.scalars(measurement, "forecast.", reading.Forecast)
		node.vector(measurement, "readout.", reading.Readout)
		node.vector(measurement, "forward.", reading.ForwardCurve)
		node.vector(measurement, "retention.", reading.ForwardRetention)
		output[1] = measurement
	}

	if reading := envelope.Manifold; reading != nil && reading.Version != uint64(projections[2].SeqIdx) {
		measurement := &projections[2]
		clear(measurement.Metrics)
		measurement.At, measurement.SeqIdx = reading.At, int64(reading.Version)
		node.scalars(measurement, "", &reading.Reading)
		output[2] = measurement
	}

	return nil
}

/* scalars writes the numerical fields of a solver's lean readout directly. */
func (node *gridNode) scalars(measurement *data.Measurement[float64], prefix string, readout any) {
	value := reflect.ValueOf(readout)

	if value.IsNil() {
		return
	}
	value = value.Elem()
	shape := value.Type()

	if node.fieldNames == nil {
		node.fieldNames = make(map[reflect.Type]map[string][]string)
	}

	if node.fieldNames[shape] == nil {
		node.fieldNames[shape] = make(map[string][]string)
	}

	labels := node.fieldNames[shape][prefix]

	if labels == nil {
		labels = make([]string, value.NumField())

		for index := range labels {
			labels[index] = prefix + shape.Field(index).Name
		}

		node.fieldNames[shape][prefix] = labels
	}

	for index := range value.NumField() {
		field := value.Field(index)
		if field.Kind() == reflect.Pointer && !field.IsNil() {
			field = field.Elem()
		}
		number := 0.0

		switch field.Kind() {
		case reflect.Float32, reflect.Float64:
			number = field.Float()
		case reflect.Int, reflect.Int32, reflect.Int64:
			number = float64(field.Int())
		case reflect.Uint, reflect.Uint32, reflect.Uint64:
			number = float64(field.Uint())
		case reflect.Bool:
			if field.Bool() {
				number = 1
			}
		default:
			continue
		}

		measurement.PutMetric(data.Metric[float64]{Label: labels[index], Raw: number})
	}
}

/* vector preserves numerical component identities while reusing their labels. */
func (node *gridNode) vector(measurement *data.Measurement[float64], prefix string, values []float64) {
	labels := node.vectors[prefix]

	for len(labels) < len(values) {
		labels = append(labels, prefix+strconv.Itoa(len(labels)))
	}
	node.vectors[prefix] = labels

	for index, value := range values {
		measurement.PutMetric(data.Metric[float64]{Label: labels[index], Raw: value})
	}
}
