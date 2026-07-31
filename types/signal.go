package types

/*
Signal measures market rows from explicit transport subscriptions and publishes
shared Thesis updates downstream.
*/
type Signal interface {
	Name() string
	Initialize(market MarketFeed, thesis *Thesis)
	Thesis() *Subscription[*Thesis]
	Close() error
}
