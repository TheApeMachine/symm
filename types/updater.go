package types

type Updater[T any] interface {
	Entity() string
	Update(T)
	Read(string) T
}
