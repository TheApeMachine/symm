package trader

/*
UIStream publishes dashboard websocket events.
*/
type UIStream interface {
	SignalScore(source string, confidence float64)
	EnginePulse(payload map[string]any)
	Status(payload map[string]any)
	Scoreboard(line, median, mad float64, targets []map[string]any)
	DecisionTrace(payload map[string]any)
}
