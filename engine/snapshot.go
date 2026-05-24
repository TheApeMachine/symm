package engine

import "time"

/*
Fresh reports whether all observer timestamps are present and within ttl,
and that cross-source skew does not exceed ttl.
*/
func (snapshot Snapshot) Fresh(now time.Time, ttl time.Duration) bool {
	if ttl <= 0 {
		return true
	}

	if snapshot.LastAt.IsZero() || snapshot.TradesAt.IsZero() || snapshot.BookAt.IsZero() {
		return false
	}

	if now.Sub(snapshot.LastAt) > ttl {
		return false
	}

	if now.Sub(snapshot.TradesAt) > ttl {
		return false
	}

	if now.Sub(snapshot.BookAt) > ttl {
		return false
	}

	earliest := minTime(snapshot.LastAt, snapshot.TradesAt, snapshot.BookAt)
	latest := maxTime(snapshot.LastAt, snapshot.TradesAt, snapshot.BookAt)

	return latest.Sub(earliest) <= ttl
}

func minTime(values ...time.Time) time.Time {
	if len(values) == 0 {
		return time.Time{}
	}

	earliest := values[0]

	for _, value := range values[1:] {
		if value.Before(earliest) {
			earliest = value
		}
	}

	return earliest
}

func maxTime(values ...time.Time) time.Time {
	if len(values) == 0 {
		return time.Time{}
	}

	latest := values[0]

	for _, value := range values[1:] {
		if value.After(latest) {
			latest = value
		}
	}

	return latest
}
