package types

import "fmt"

/*
Measurement groups named boundary metrics from one source. Reducers should
project a Frame into Measurement only when leaving the numeric hot path.
*/
type Measurement struct {
	ID      string                      `json:"id"`
	Source  string                      `json:"source"`
	At      int64                       `json:"at"`
	From    int64                       `json:"from"`
	Metrics map[string]*Metric[float64] `json:"metrics"`
	Err     error                       `json:"-"`
}

func NewMeasurement(id string, source string) *Measurement {
	return &Measurement{
		ID:      id,
		Source:  source,
		Metrics: make(map[string]*Metric[float64]),
	}
}

func (measurement *Measurement) Put(name string, metric *Metric[float64]) {
	if measurement.Metrics == nil {
		measurement.Metrics = make(map[string]*Metric[float64])
	}

	measurement.Metrics[name] = metric
}

/*
AddMetrics appends the given boundary metrics in one expression, indexing each
one by the name the metric carries. The projection from a numeric Frame to a
Measurement therefore stays a single chained statement at the hot-path exit.
*/
func (measurement *Measurement) AddMetrics(metrics ...*Metric[float64]) *Measurement {
	if measurement == nil {
		return nil
	}

	for _, metric := range metrics {
		if metric == nil || metric.Name == "" {
			continue
		}

		measurement.Put(metric.Name, metric)
	}

	return measurement
}

func (measurement *Measurement) Metric(name string) *Metric[float64] {
	metric, found := measurement.Metrics[name]

	if !found {
		measurement.Err = fmt.Errorf("measurement: metric %q is missing", name)
	}

	return metric
}

func (measurement *Measurement) Error() string {
	if measurement.Err == nil {
		return ""
	}

	return measurement.Err.Error()
}
