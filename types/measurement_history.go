package types

/*
AppendMeasurements retains signal observations for the complete decision cycle.
*/
func (thesis *Thesis) AppendMeasurements(
	source SourceType,
	measurements ...*Measurement,
) {
	if thesis == nil || len(measurements) == 0 {
		return
	}

	thesis.measurementMu.Lock()
	defer thesis.measurementMu.Unlock()

	found, ok := thesis.Measurements.Load(source)

	if !ok {
		thesis.Measurements.Store(source, measurements)
		return
	}

	rows, ok := found.([]*Measurement)

	if !ok {
		panic("thesis: measurement source contains an unexpected value type")
	}

	thesis.Measurements.Store(source, append(rows, measurements...))
}
