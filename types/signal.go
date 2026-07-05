package types

type Signal[T any] interface {
	IngestRoles() []string
	Categories() []CategoryType
	Measure(T, *CrossSection) ([]*Measurement, error)
}
