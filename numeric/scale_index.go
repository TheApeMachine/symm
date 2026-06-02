package numeric

/*
ScaleIndex multiplies the running pipeline output by one selected observation.
*/
type ScaleIndex struct {
	index int
}

/*
NewScaleIndex scales out by values[index].
*/
func NewScaleIndex(index int) *ScaleIndex {
	return &ScaleIndex{index: index}
}

func (scale *ScaleIndex) Next(out float64, values ...float64) (float64, error) {
	if scale.index >= len(values) || values[scale.index] <= 0 {
		return 0, nil
	}

	if out <= 0 {
		return values[scale.index], nil
	}

	return out * values[scale.index], nil
}

func (scale *ScaleIndex) Reset() error {
	return nil
}
