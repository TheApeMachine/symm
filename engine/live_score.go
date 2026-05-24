package engine

/*
LiveReading is the strongest symbol-level score currently held in a signal track.
*/
type LiveReading struct {
	Symbol string
	Score  float64
}

/*
LiveScoreReader exposes live dashboard scores from signal track stores.
Scores update every scan, not only when a measurement is enqueued for the trader.
*/
type LiveScoreReader interface {
	LiveScore() float64
	PeakReading() LiveReading
}

/*
MeanConfidenceReader exposes the mean normalized confidence across the latest scan set.
*/
type MeanConfidenceReader interface {
	MeanConfidence() float64
}
