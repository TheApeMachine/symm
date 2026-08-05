package types

import "slices"

/*
AppendMeasurements retains the most recent signal observations for one source.
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

	retained := append(rows, measurements...)

	/*
		One source's rows cover every symbol it measured, so the retained
		window is scaled by the symbols present rather than bounded at the
		per-symbol retention, which would otherwise evict a quiet symbol's
		only reading as soon as a busy one filled the buffer.
	*/
	bound := EvidenceRetention * max(1, distinctSymbols(retained))

	if overflow := len(retained) - bound; overflow > 0 {
		retained = slices.Delete(retained, 0, overflow)
	}

	thesis.Measurements.Store(source, retained)
}

func distinctSymbols(measurements []*Measurement) int {
	symbols := make(map[string]struct{})

	for _, measurement := range measurements {
		if measurement != nil {
			symbols[measurement.Symbol] = struct{}{}
		}
	}

	return len(symbols)
}
