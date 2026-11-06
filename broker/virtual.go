package broker

import (
	"github.com/shopspring/decimal"
	"github.com/theapemachine/symm/network"
)

/*
BindVirtual wires book prices and Price.afterFee into the virtual private
client so wallet cash matches the same fee math the desk marks with.
*/
func BindVirtual(virtual *network.Virtual, price *Price) {
	if virtual == nil || price == nil {
		return
	}

	virtual.UseFills(price)
	virtual.UseQuoteSettle(func(
		symbol string,
		gross decimal.Decimal,
		buy bool,
	) (cash, feeAmt decimal.Decimal) {
		fee := price.Fee(symbol)
		if fee == nil {
			zero := decimal.Zero
			fee = &zero
		}

		dir := BUY
		if !buy {
			dir = SELL
		}

		net := price.afterFee(&gross, fee, dir)
		if net == nil {
			return gross, decimal.Zero
		}

		if buy {
			return *net, net.Sub(gross)
		}

		return *net, gross.Sub(*net)
	})
}
