package trader

/*
PriceReader supplies the latest trade price for prediction settlement.
*/
type PriceReader interface {
	Last(symbol string) (float64, bool)
}
