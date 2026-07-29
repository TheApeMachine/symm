package types

import "sync"

/*
StoreMeasurements replaces the current per-source measurement batch on Thesis.
Signals own complete source snapshots for each cut, so the latest batch wins.
*/
func StoreMeasurements(thesis *Thesis, measurements []*Measurement) {
	if thesis == nil || len(measurements) == 0 {
		return
	}

	batches := make(map[SourceType][]*Measurement)

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		batches[measurement.Source] = append(
			batches[measurement.Source],
			measurement,
		)
	}

	for source, batch := range batches {
		thesis.Measurements.Store(source, batch)
	}
}

/*
RangeMeasurements walks every retained measurement row currently stored on Thesis.
*/
func RangeMeasurements(thesis *Thesis, visit func(*Measurement) bool) {
	if thesis == nil || thesis.Measurements == nil || visit == nil {
		return
	}

	thesis.Measurements.Range(func(_, value any) bool {
		switch rows := value.(type) {
		case []*Measurement:
			for _, measurement := range rows {
				if measurement != nil && !visit(measurement) {
					return false
				}
			}
		case *Measurement:
			if rows != nil && !visit(rows) {
				return false
			}
		}

		return true
	})
}

/*
snapshotMeasurements copies the current per-source measurement membership for cuts.
*/
func snapshotMeasurements(thesis *Thesis) map[string][]*Measurement {
	if thesis == nil || thesis.Measurements == nil {
		return nil
	}

	out := make(map[string][]*Measurement)
	thesis.Measurements.Range(func(key, value any) bool {
		source, ok := key.(SourceType)
		if !ok {
			return true
		}

		rows, ok := value.([]*Measurement)
		if !ok || len(rows) == 0 {
			return true
		}

		out[string(source)] = append([]*Measurement(nil), rows...)
		return true
	})

	return out
}

/*
snapshotMap copies one sync.Map into a plain map for cut checkpointing.
*/
func snapshotMap(source *sync.Map) map[string]any {
	if source == nil {
		return nil
	}

	out := make(map[string]any)
	source.Range(func(key, value any) bool {
		name, ok := key.(string)
		if ok {
			out[name] = value
		}
		return true
	})

	return out
}
