package types

import (
	"fmt"

	"github.com/theapemachine/errnie"
)

/*
Measurement groups named metrics from one source. It implements Input and
Output so an algorithm can consume a whole reading, while Metric still lets a
later stage take one quantity as Input.
*/
type Measurement struct {
	ID      string                      `json:"id"`
	Source  string                      `json:"source"`
	At      int64                       `json:"at"`
	From    int64                       `json:"from"`
	Metrics map[string]*Metric[float64] `json:"metrics"`
	Err     error                       `json:"err"`
}

/*
NewMeasurement returns an empty reading for one source.
*/
func NewMeasurement(id string, source string) *Measurement {
	return &Measurement{
		ID:      id,
		Source:  source,
		Metrics: make(map[string]*Metric[float64]),
	}
}

/*
Put stores one named metric.
*/
func (measurement *Measurement) Put(name string, metric *Metric[float64]) {
	if measurement.Metrics == nil {
		measurement.Metrics = map[string]*Metric[float64]{}
	}

	measurement.Metrics[name] = metric
}

/*
Metric returns one named quantity as Input so it can feed the next stage.
*/
func (measurement *Measurement) Metric(name string) *Metric[float64] {
	metric, found := measurement.Metrics[name]

	if !found {
		measurement.Err = errnie.Error(errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("measurement: metric %q is missing", name),
			nil,
		))
	}

	return metric
}

/*
Error reports a missing metric or an earlier failure.
*/
func (measurement *Measurement) Error() string {
	return measurement.Err.Error()
}
