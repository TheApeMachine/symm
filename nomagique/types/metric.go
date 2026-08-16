package types

/*
Metric is boundary metadata for one measured quantity. It is deliberately not a
Primitive input; numeric composition remains Frame-only.
*/
type Metric[Value comparable] struct {
	Value      Value      `json:"value"`
	Normalized Value      `json:"normalized"`
	Descriptor Descriptor `json:"descriptor"`
	Err        error      `json:"-"`
}

func NewMetric[Value comparable](
	value Value,
	descriptor Descriptor,
) *Metric[Value] {
	return &Metric[Value]{
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
