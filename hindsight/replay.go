package hindsight

import "sort"

/*
CausalOrdering is the authoritative horizontal order of the Hindsight tape:
Run → CaptureSequence → EnvelopeOrdinal → Component StateVersion. It is used by
the replay cursor to advance the tape deterministically, never by wall-clock
time. Each field is strictly "later" than the previous, so the total order is
total and stable across two replays of the same capture set.
*/
type CausalOrdering struct {
	Run                   RunID
	Sequence              CaptureSequence
	Ordinal               uint64
	ComponentStateVersion uint64
}

/*
Less reports whether o strictly precedes other under the causal order. The
comparison is lexicographic over Run, Sequence, Ordinal, then version.
*/
func (o CausalOrdering) Less(other CausalOrdering) bool {
	if o.Run != other.Run {
		return o.Run < other.Run
	}

	if o.Sequence != other.Sequence {
		return o.Sequence < other.Sequence
	}

	if o.Ordinal != other.Ordinal {
		return o.Ordinal < other.Ordinal
	}

	return o.ComponentStateVersion < other.ComponentStateVersion
}

/*
ReplayCursor is the fast-forward state machine over an immutable causal tape.
It steps captures, envelopes, and semantic state transitions in causal order —
as fast as computation permits, with no sleeps — and stops at the next marked
event. It never re-executes trading: it is an inspection cursor, distinct from
Historical Witness data.
*/
type ReplayCursor struct {
	run       RunID
	positions []CausalOrdering
	position  CausalOrdering
	index     int
	records   []ReplayRecord
}

/*
ReplayRecord is one ordered step of the replay tape: the causal position it
occupies, and the marker class (if any) that makes it an "interesting moment".
The marker is a stable name, not a strategy judgment.
*/
type ReplayRecord struct {
	Position CausalOrdering
	Markers  []string
}

/*
NewReplayCursor builds a cursor over the given ordered records. Records need not
already be sorted: the cursor sorts them into causal order so stepping is
deterministic regardless of how the caller assembled them.
*/
func NewReplayCursor(run RunID, records []ReplayRecord) *ReplayCursor {
	sorted := append([]ReplayRecord(nil), records...)

	sort.SliceStable(sorted, func(left, right int) bool {
		return sorted[left].Position.Less(sorted[right].Position)
	})

	return &ReplayCursor{
		run:     run,
		records: sorted,
		index:   -1,
	}
}

/*
Step advances one causal position and returns that record, or false when the
tape is exhausted. There is no wall-clock delay: advance is the only cost.
*/
func (cursor *ReplayCursor) Step() (ReplayRecord, bool) {
	if cursor == nil || cursor.index+1 >= len(cursor.records) {
		return ReplayRecord{}, false
	}

	cursor.index++
	cursor.position = cursor.records[cursor.index].Position

	return cursor.records[cursor.index], true
}

/*
FastForward advances repeatedly until the position is at or past the target
CaptureSequence, returning the records passed over so callers can replay the
causal deltas they skipped. It is bounded only by computation.
*/
func (cursor *ReplayCursor) FastForward(target CaptureSequence) []ReplayRecord {
	if cursor == nil {
		return nil
	}

	skipped := make([]ReplayRecord, 0)

	for {
		record, ok := cursor.Step()

		if !ok {
			return skipped
		}

		if record.Position.Sequence >= target {
			// The step just advanced past the target; rewind one so the cursor
			// rests on the first record meeting the target.
			cursor.index--
			cursor.position = cursor.records[cursor.index].Position

			return skipped
		}

		skipped = append(skipped, record)
	}
}

/*
NextMarked advances until it lands on a record carrying a marker, or exhausts
the tape. It returns the marked record, or false when no further marker exists.
*/
func (cursor *ReplayCursor) NextMarked() (ReplayRecord, bool) {
	if cursor == nil {
		return ReplayRecord{}, false
	}

	for {
		record, ok := cursor.Step()

		if !ok {
			return ReplayRecord{}, false
		}

		if len(record.Markers) > 0 {
			return record, true
		}
	}
}

/*
Position returns the cursor's current causal position (zero before the first
step).
*/
func (cursor *ReplayCursor) Position() CausalOrdering {
	if cursor == nil {
		return CausalOrdering{}
	}

	return cursor.position
}
