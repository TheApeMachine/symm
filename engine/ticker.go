package engine

/*
Ticker drains one non-blocking market or telemetry message per call.
*/
type Ticker interface {
	Tick() bool
}

/*
DrainTicker drains up to limit pending messages and returns how many it processed.
*/
type DrainTicker interface {
	Drain(limit int) int
}
