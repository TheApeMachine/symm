package types

/*
Signal conditions one market input into numerical measurements. IngestRoles is
retained while the current feed wiring uses it to route source streams; market
interpretations are deliberately absent because they belong to logic.
*/
type Signal[T any] interface {
	IngestRoles() []string
	Measure(T, *CrossSection) ([]*Measurement, error)
}
