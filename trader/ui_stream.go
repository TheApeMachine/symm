package trader

/*
UIStream publishes dashboard websocket events.
*/
type UIStream interface {
	SignalScore(source string, confidence float64)
	EnginePulse(payload map[string]any)
}
