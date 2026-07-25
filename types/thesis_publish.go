package types

/*
measureKey identifies one published source×symbol row for O(1) Thesis.Publish.
*/
type measureKey struct {
	source SourceType
	symbol string
}

/*
Publish upserts rows onto the Thesis by source×symbol. Incoming Metrics replace
the prior map for that identity so book-path publishes stay O(incoming) under
the publish lock.
*/
func (thesis *Thesis) Publish(source SourceType, rows []*Measurement) {
	if thesis == nil || len(rows) == 0 {
		return
	}

	incoming := make(map[measureKey]*Measurement, len(rows))
	order := make([]measureKey, 0, len(rows))

	for _, row := range rows {
		if row == nil || row.Symbol == "" {
			continue
		}

		row.Source = source
		key := measureKey{source, row.Symbol}

		if _, seen := incoming[key]; !seen {
			order = append(order, key)
		}

		incoming[key] = row
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

			thesis.index[measureKey{row.Source, row.Symbol}] = slot
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
}

/*
Replace publishes a complete per-symbol surface for source: every prior row from
that source for symbols present in rows is dropped, then the incoming identities
are written. Book-only cuts therefore cannot keep a prior trade metric alive.
*/
func (thesis *Thesis) Replace(source SourceType, rows []*Measurement) {
	if thesis == nil || len(rows) == 0 {
		return
	}

	symbols := make(map[string]struct{}, len(rows))

	for _, row := range rows {
		if row == nil || row.Symbol == "" {
			continue
		}

		symbols[row.Symbol] = struct{}{}
	}

	if len(symbols) == 0 {
		return
	}

	thesis.publish.Lock()
	defer thesis.publish.Unlock()

	kept := thesis.Measurements[:0]

	for _, row := range thesis.Measurements {
		if row == nil {
			continue
		}

		if row.Source == source {
			if _, drop := symbols[row.Symbol]; drop {
				continue
			}
		}

		kept = append(kept, row)
	}

	thesis.Measurements = kept
	thesis.rebuildIndex()

	if thesis.index == nil {
		thesis.index = make(map[measureKey]int, len(rows))
	}

	incoming := make(map[measureKey]*Measurement, len(rows))
	order := make([]measureKey, 0, len(rows))

	for _, row := range rows {
		if row == nil || row.Symbol == "" {
			continue
		}

		row.Source = source
		key := measureKey{source, row.Symbol}

		if _, seen := incoming[key]; !seen {
			order = append(order, key)
		}

		incoming[key] = row
	}

	for _, key := range order {
		row := incoming[key]
		thesis.index[key] = len(thesis.Measurements)
		thesis.Measurements = append(thesis.Measurements, row)
	}
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

		thesis.index[measureKey{row.Source, row.Symbol}] = slot
	}
}
