package market

/*
BookSequence enforces Kraken's snapshot-before-delta protocol for one symbol's book
feed. Deltas received before the first snapshot are dropped so the maintained book
is never seeded from a partial delta before the first snapshot.
*/
type BookSequence struct {
	hasSnapshot bool
}

func (sequence *BookSequence) CanAccept(update BookUpdate) bool {
	if update.IsSnapshot() {
		return true
	}

	return sequence.hasSnapshot
}

func (sequence *BookSequence) AdmitSnapshot() {
	sequence.hasSnapshot = true
}

// Accepts is kept for callers that only need a non-mutating predicate.
func (sequence *BookSequence) Accepts(update BookUpdate) bool {
	return sequence.CanAccept(update)
}
