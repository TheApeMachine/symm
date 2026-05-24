package engine

/*
Ticker drains one non-blocking market or telemetry message per call.
The orchestrator loops until every ticker returns false.
*/
type Ticker interface {
	Tick() bool
}
