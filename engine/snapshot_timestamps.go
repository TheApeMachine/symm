package engine

/*
snapshotHasTimestamps reports whether ingest populated any observer timestamps.
Freshness gating applies only when at least one timestamp is present.
*/
func snapshotHasTimestamps(snapshot Snapshot) bool {
	return !snapshot.LastAt.IsZero() ||
		!snapshot.TradesAt.IsZero() ||
		!snapshot.BookAt.IsZero()
}
