package types

/*
measureKey identifies one published row for O(1) Thesis.Publish upsert.
*/
type measureKey struct {
	source SourceType
	metric MetricType
	side   MeasurementSide
	symbol string
}

/*
Publish upserts rows onto the Thesis by source/metric/side/symbol. Existing
identities are replaced through an index so book-path publishes stay O(incoming)
under the publish lock instead of rescanning the full measurement bag. For each
symbol present in the incoming batch, prior rows from the same source that are
absent from this publish are retracted so a book-only cut cannot keep a prior
trade metric alive on the durable surface.
*/
func (thesis *Thesis) Publish(source SourceType, rows []*Measurement) {
	if thesis == nil || len(rows) == 0 {
		return
	}

	incoming := make(map[measureKey]*Measurement, len(rows))
	order := make([]measureKey, 0, len(rows))
	symbols := make(map[string]struct{}, len(rows))

	for _, row := range rows {
		if row == nil {
			continue
		}

		row.Source = source
		key := measureKey{source, row.Metric, row.Side, row.Symbol}

		if _, seen := incoming[key]; !seen {
			order = append(order, key)
		}

		incoming[key] = row
		symbols[row.Symbol] = struct{}{}
	}

	if len(order) == 0 {
		return
	}

	thesis.publish.Lock()
	defer thesis.publish.Unlock()

	if thesis.index == nil {
		thesis.index = make(map[measureKey]int, len(thesis.Measurements)+len(order))
		for slot, row := range thesis.Measurements {
			if row == nil {
				continue
			}

			thesis.index[measureKey{
				row.Source, row.Metric, row.Side, row.Symbol,
			}] = slot
		}
	}

	for _, key := range order {
		row := incoming[key]

		if slot, ok := thesis.index[key]; ok &&
			slot < len(thesis.Measurements) {
			thesis.Measurements[slot] = row
			continue
		}

		thesis.index[key] = len(thesis.Measurements)
		thesis.Measurements = append(thesis.Measurements, row)
	}

	thesis.retractAbsent(source, symbols, incoming)
}

/*
retractAbsent drops same-source rows for batch symbols whose identities were
not included in the current publish. Callers hold thesis.publish.
*/
func (thesis *Thesis) retractAbsent(
	source SourceType,
	symbols map[string]struct{},
	incoming map[measureKey]*Measurement,
) {
	kept := thesis.Measurements[:0]
	changed := false

	for _, row := range thesis.Measurements {
		if row == nil {
			changed = true
			continue
		}

		if row.Source != source {
			kept = append(kept, row)
			continue
		}

		if _, inBatch := symbols[row.Symbol]; !inBatch {
			kept = append(kept, row)
			continue
		}

		key := measureKey{source, row.Metric, row.Side, row.Symbol}

		if _, present := incoming[key]; present {
			kept = append(kept, row)
			continue
		}

		changed = true
	}

	if !changed && len(kept) == len(thesis.Measurements) {
		return
	}

	thesis.Measurements = kept
	thesis.rebuildIndex()
}

/*
rebuildIndex reconstructs the Publish upsert map from the live measurement slice.
*/
func (thesis *Thesis) rebuildIndex() {
	if thesis == nil {
		return
	}

	if len(thesis.Measurements) == 0 {
		thesis.index = nil
		return
	}

	thesis.index = make(map[measureKey]int, len(thesis.Measurements))

	for slot, row := range thesis.Measurements {
		if row == nil {
			continue
		}

		thesis.index[measureKey{
			row.Source, row.Metric, row.Side, row.Symbol,
		}] = slot
	}
}
