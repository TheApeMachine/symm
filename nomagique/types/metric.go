package types

/*
Metric is boundary metadata for one measured quantity. It carries its own name
so a producer can project several Frame slots into one Measurement with a
single AddMetrics expression. It is deliberately not a Primitive input; numeric
composition remains Frame-only.
*/
type Metric[Value comparable] struct {
	Name       string     `json:"name"`
	Value      Value      `json:"value"`
	Normalized Value      `json:"normalized"`
	Descriptor Descriptor `json:"descriptor"`
	Err        error      `json:"-"`
}

func NewMetric[Value comparable](
	name string,
	value Value,
	descriptor Descriptor,
) *Metric[Value] {
	return &Metric[Value]{
		Name:       name,
		Value:      value,
		Descriptor: descriptor,
	}
}

func (metric *Metric[Value]) Unit() Unit {
	if metric == nil {
		return UnitUnknown
	}

	return metric.Descriptor.Unit
}

func (metric *Metric[Value]) Timescale() Timescale {
	if metric == nil {
		return TimescaleUnknown
	}

	return metric.Descriptor.Timescale
}

func (metric *Metric[Value]) Error() error {
	if metric == nil {
		return nil
	}

	return metric.Err
}
