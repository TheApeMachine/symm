package types

/*
Metric is boundary metadata for one measured quantity. It carries its own name
so a producer can project several Frame slots into one Measurement with a
single AddMetrics expression. Raw is the measured value; Normalized, when set,
is the value projected into the competing common [0,1] domain the hypothesis
separation contract requires.
*/
type Metric[Value comparable] struct {
	Name       string     `json:"name"`
	Raw        Value      `json:"raw"`
	Normalized *Value     `json:"normalized,omitempty"`
	Unit       Unit       `json:"unit,omitempty"`
	Timescale  Timescale  `json:"timescale,omitempty"`
	Descriptor Descriptor `json:"descriptor"`
	Err        error      `json:"-"`
}

func NewMetric[Value comparable](
	name string,
	raw Value,
	descriptor Descriptor,
) *Metric[Value] {
	return &Metric[Value]{
		Name:       name,
		Raw:        raw,
		Unit:       descriptor.Unit,
		Timescale:  descriptor.Timescale,
		Descriptor: descriptor,
	}
}

/*
NewNormalizedMetric builds one metric with both its raw reading and its
normalized competing value.
*/
func NewNormalizedMetric[Value comparable](
	name string,
	raw Value,
	normalized Value,
	descriptor Descriptor,
) *Metric[Value] {
	normalizedValue := normalized

	return &Metric[Value]{
		Name:       name,
		Raw:        raw,
		Normalized: &normalizedValue,
		Unit:       descriptor.Unit,
		Timescale:  descriptor.Timescale,
		Descriptor: descriptor,
	}
}

func (metric *Metric[Value]) Error() error {
	if metric == nil {
		return nil
	}

	return metric.Err
}
