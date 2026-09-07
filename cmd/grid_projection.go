package cmd

import (
	"math"
	"reflect"
	"strconv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

/*
project reuses three Measurement objects per key for scalar logic readouts.
Scalars are reflected from the declared readout structs, so adding a numeric
field cannot silently omit it. Manifold spectral components and particle
moments are projected once per producer version, without retaining grid fields.
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
		node.manifold(measurement, reading)
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

/* manifold writes spectral power/phase and population moments into learning. */
func (node *gridNode) manifold(measurement *data.Measurement[float64], reading *types.ManifoldState) {
	components := 2 * len(reading.Modes)

	if cap(node.waveComponents) < components {
		node.waveComponents = make([]float64, components)
	}

	node.waveComponents = node.waveComponents[:components]
	powers, phases := node.waveComponents[:len(reading.Modes)], node.waveComponents[len(reading.Modes):]

	for index, mode := range reading.Modes {
		real, imag := float64(mode.Real), float64(mode.Imag)
		powers[index] = real*real + imag*imag
		phases[index] = math.Atan2(imag, real)
	}

	node.vector(measurement, "wavespace.power.", powers)
	node.vector(measurement, "wavespace.phase.", phases)
	measurement.PutMetric(data.Metric[float64]{Label: "particles.count", Raw: float64(reading.N)})

	if reading.N == 0 {
		return
	}

	var energy, heat [2]float64
	var velocity [3]float64

	for index := 0; index < reading.N; index++ {
		side := reading.TokenIDs[index] & 1
		energy[side] += float64(reading.Energy[index])
		heat[side] += float64(reading.Heat[index])

		for axis := range velocity {
			velocity[axis] += float64(reading.Vel[index*3+axis])
		}
	}

	for _, metric := range [...]data.Metric[float64]{
		{Label: "particles.energy.bid", Raw: energy[0]},
		{Label: "particles.energy.ask", Raw: energy[1]},
		{Label: "particles.heat.bid", Raw: heat[0]},
		{Label: "particles.heat.ask", Raw: heat[1]},
		{Label: "particles.vel.mean_x", Raw: velocity[0] / float64(reading.N)},
		{Label: "particles.vel.mean_y", Raw: velocity[1] / float64(reading.N)},
		{Label: "particles.vel.mean_z", Raw: velocity[2] / float64(reading.N)},
	} {
		measurement.PutMetric(metric)
	}
}
