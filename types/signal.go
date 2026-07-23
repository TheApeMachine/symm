package types

/*
Signal receives market rows on channels, measures them, and yields batches via
Measure. HandleTicker/HandleBook/HandleTrade enqueue for MeasureLoop; Crypto
uses offer() for latest-wins so the live websocket path never blocks. BindIngress
attaches the WaitGroup Done each Measure finishes. Run starts MeasureLoop.
*/
type Signal interface {
	Run()
	Name() string
}
