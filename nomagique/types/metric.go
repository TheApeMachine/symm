package types

/*
Metric is one measured quantity. It implements Input and Output so a raw
reading can feed the next stage, while still carrying unit, timescale, and an
optional normalized form.
*/
type Metric[T comparable] struct {
	Value      Value[T]   `json:"value"`
	Normalized Value[T]   `json:"normalized"`
	Descriptor Descriptor `json:"descriptor"`
	Err        error      `json:"err"`
}

/*
NewMetric returns a ready metric. Normalized stays absent until Normalize.
*/
func NewMetric[T comparable](
	value Value[T], descriptor Descriptor,
) *Metric[T] {
	return &Metric[T]{
		Value:      value,
		Descriptor: descriptor,
	}
}

/*
Unit returns the native unit of the raw value.
*/
func (metric *Metric[T]) Unit() Unit {
	return metric.Descriptor.Unit
}

/*
Timescale returns the temporal grain of the raw value.
*/
func (metric *Metric[T]) Timescale() Timescale {
	return metric.Descriptor.Timescale
}

/*
Error reports a failure on this metric.
*/
func (metric *Metric[T]) Error() error {
	return metric.Err
}
