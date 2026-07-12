package trader

/*
Level3Scheduler owns a deduplicated FIFO of symbols whose ordered L3 changes
are ready for one manifold advancement.
*/
type Level3Scheduler struct {
	queued  map[string]struct{}
	symbols []string
}

/*
NewLevel3Scheduler creates an empty owner-local scheduler.
*/
func NewLevel3Scheduler() *Level3Scheduler {
	return &Level3Scheduler{queued: map[string]struct{}{}}
}

/*
Mark appends symbol once, preserving the first-ready order.
*/
func (scheduler *Level3Scheduler) Mark(symbol string) bool {
	if symbol == "" {
		return false
	}

	if _, exists := scheduler.queued[symbol]; exists {
		return false
	}

	scheduler.queued[symbol] = struct{}{}
	scheduler.symbols = append(scheduler.symbols, symbol)
	return true
}

/*
Remove withdraws a symbol whose most recent observation invalidated its state.
*/
func (scheduler *Level3Scheduler) Remove(symbol string) bool {
	if _, exists := scheduler.queued[symbol]; !exists {
		return false
	}

	delete(scheduler.queued, symbol)

	for index, queued := range scheduler.symbols {
		if queued != symbol {
			continue
		}

		copy(scheduler.symbols[index:], scheduler.symbols[index+1:])
		scheduler.symbols = scheduler.symbols[:len(scheduler.symbols)-1]
		return true
	}

	return false
}

/*
Next returns one fair symbol and makes it eligible to be queued at the tail by
events that arrive during its advancement.
*/
func (scheduler *Level3Scheduler) Next() (string, bool) {
	if len(scheduler.symbols) == 0 {
		return "", false
	}

	symbol := scheduler.symbols[0]
	copy(scheduler.symbols, scheduler.symbols[1:])
	scheduler.symbols = scheduler.symbols[:len(scheduler.symbols)-1]
	delete(scheduler.queued, symbol)

	return symbol, true
}

/*
Len returns the number of independently ready symbols.
*/
func (scheduler *Level3Scheduler) Len() int {
	return len(scheduler.symbols)
}
