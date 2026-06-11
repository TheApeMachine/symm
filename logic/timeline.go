package logic

/*
sliceTimelineAfter returns measurements strictly after matchIndex.

Tree.Evaluate passes the full ring to every top-level branch; slices are only
applied when descending parent→child within one branch path. Sibling branches and
top-level branches always restart from the full measurements slice.

matchIndex < 0 means no temporal anchor (e.g. holding / entry_branch gates) and
the child timeline stays the full slice.
*/
func sliceTimelineAfter(measurements []Measurement, matchIndex int) []Measurement {
	if matchIndex < 0 {
		return measurements
	}

	if matchIndex >= len(measurements)-1 {
		return nil
	}

	return measurements[matchIndex+1:]
}

func maxMatchIndex(current int, candidate int) int {
	if candidate > current {
		return candidate
	}

	return current
}
