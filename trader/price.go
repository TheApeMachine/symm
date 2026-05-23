package trader

/*
PriceReader supplies live last-trade prices for paper entries and exits.
*/
type PriceReader interface {
	Last(symbol string) (float64, bool)
}

/*
QuoteReader supplies live quotes for paper entries, exits, and UI ticks.
*/
type QuoteReader interface {
	PriceReader
	Quote(symbol string) (last, bid, ask, changePct float64, ok bool)
	Timestamp(symbol string) (string, bool)
}
