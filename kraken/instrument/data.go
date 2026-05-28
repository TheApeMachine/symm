package instrument

import (
	"strconv"

	"github.com/theapemachine/symm/kraken/asset"
)

/*
Data is one tradable pair from the Kraken WebSocket v2 instrument channel.
*/
type Data struct {
	Symbol         string  `json:"symbol"`
	Base           string  `json:"base"`
	Quote          string  `json:"quote"`
	Status         string  `json:"status"`
	CostMin        float64 `json:"cost_min"`
	QtyMin         float64 `json:"qty_min"`
	QtyPrecision   int     `json:"qty_precision"`
	PricePrecision int     `json:"price_precision"`
	CostPrecision  int     `json:"cost_precision"`
	PriceIncrement float64 `json:"price_increment"`
}

/*
Pair maps instrument channel data into the shared asset pair shape.
*/
func (data Data) Pair() asset.Pair {
	return asset.Pair{
		Wsname:       data.Symbol,
		Altname:      data.Symbol,
		Base:         data.Base,
		Quote:        data.Quote,
		Costmin:      strconv.FormatFloat(data.CostMin, 'f', -1, 64),
		CostDecimals: data.CostPrecision,
		PairDecimals: data.PricePrecision,
		LotDecimals:  data.QtyPrecision,
		Status:       data.Status,
	}
}
