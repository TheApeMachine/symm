package kraken

/*
Side identifies which half of an order book a level belongs to.
*/
type Side string

const (
	SideBid Side = "bid"
	SideAsk Side = "ask"
)
