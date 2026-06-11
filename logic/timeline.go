package logic

/*
sliceTimelineAfter returns measurements strictly after matchIndex. A negative
matchIndex leaves the timeline intact for conditions that do not anchor time.
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
