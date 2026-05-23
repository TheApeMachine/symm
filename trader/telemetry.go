package trader

/*
Publisher emits flat JSON telemetry events for the UI hub.
*/
type Publisher interface {
	Emit(event map[string]any)
}

type noopPublisher struct{}

func (noopPublisher) Emit(map[string]any) {}

func NoopPublisher() Publisher {
	return noopPublisher{}
}
