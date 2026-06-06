package reasoning

/*
Subject is what a leaf predicate observes. It widens the vocabulary past category
signals so a thought can reason about price action, participation, the clock, and
the trade's own life — the things a trader actually watches.
*/
type Subject uint8

const (
	SubjectNone     Subject = iota
	SubjectSignal           // a microstructure category's strength (pairs with Category + Unit snr|confidence)
	SubjectRegime           // the price-action regime (target in Regime)
	SubjectPosition         // the trade's lifecycle (target in Lifecycle: not_holding|holding|has_started|has_continued|has_ended)
	SubjectPrice            // last traded price (Unit percentage for relative moves)
	SubjectVolume           // traded volume
	SubjectSpread           // quoted spread (bps)
	SubjectElapsed          // time held in the current position (Unit time_minutes|time_seconds)
)
