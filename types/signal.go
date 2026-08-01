package types

/*
Signal measures market rows from explicit transport subscriptions and publishes
shared Thesis updates downstream.
*/
type Signal interface {
	Name() string
	Subscribe(string, *Subscription[any]) *Subscription[any]
	Close() error
}
