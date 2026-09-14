package tables

func (w *Writer) AddPosition(row PositionRow) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.positions = append(w.positions, row)
}

func (w *Writer) AddOutcome(row OutcomeRow) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.outcomes = append(w.outcomes, row)
}
