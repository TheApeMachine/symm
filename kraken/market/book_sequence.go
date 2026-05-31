package market

/*
BookSequence enforces Kraken's snapshot-before-delta protocol for one symbol's book
feed. Deltas received before the first snapshot are dropped so the maintained book
is never seeded from a partial delta before the first snapshot.
*/
type BookSequence struct {
	hasSnapshot bool
}

func (sequence *BookSequence) Accepts(update BookUpdate) bool {
	if update.IsSnapshot() {
		sequence.hasSnapshot = true

		return true
	}

	return sequence.hasSnapshot
}
