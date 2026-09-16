package position

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

// Regulator owns inventory and one pending order. Only cumulative venue fills
// change inventory; submission and acknowledgement are not fills.
type Regulator struct {
	Symbol      string
	Quantity    *decimal.Decimal
	Basis       *decimal.Decimal
	EntryFee    *decimal.Decimal
	Realized    *decimal.Decimal
	Pending     *spot.AddOrderRequest
	OrderID     string
	LastOrderID string
	filled      *decimal.Decimal
	cost        *decimal.Decimal
	fee         *decimal.Decimal
}

func NewRegulator(symbol string) *Regulator {
	return &Regulator{
		Symbol:   symbol,
		Quantity: decimal.NewFromInt64(0),
		Basis:    decimal.NewFromInt64(0),
		EntryFee: decimal.NewFromInt64(0),
		Realized: decimal.NewFromInt64(0),
	}
}

func (regulator *Regulator) Begin(request *spot.AddOrderRequest) error {
	if regulator.Pending != nil {
		return errnie.Error(errnie.Err(errnie.Conflict, "position: order already pending", nil))
	}

	if request == nil || request.Pair != regulator.Symbol || request.ClOrdId == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "position: identified order required", nil))
	}

	regulator.Pending = request
	regulator.OrderID = ""
	regulator.filled = decimal.NewFromInt64(0)
	regulator.cost = decimal.NewFromInt64(0)
	regulator.fee = decimal.NewFromInt64(0)
	return nil
}

// Reconcile applies cumulative deltas once, retaining exact basis and fee
// remainders after partial sales. A terminal report releases the pending order.
func (regulator *Regulator) Reconcile(report kraken.ExecutionData) error {
	if report.OrderID != "" && report.OrderID == regulator.LastOrderID {
		return nil
	}

	if regulator.Pending == nil || (report.OrderID != regulator.OrderID && report.ClientOrderID != regulator.Pending.ClOrdId) {
		return errnie.Error(errnie.Err(errnie.Validation, "position: execution does not identify pending order", nil))
	}

	if report.CumQty != nil && report.CumQty.Sign() > 0 {
		if err := regulator.apply(report); err != nil {
			return err
		}
	}

	switch report.OrderStatus {
	case "filled", "canceled", "expired", "rejected":
		regulator.LastOrderID = report.OrderID
		regulator.Pending = nil
	}

	return nil
}

func (regulator *Regulator) apply(report kraken.ExecutionData) error {
	if report.CumCost == nil || report.FeeUsdEquiv == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "position: cumulative cost and quote fee required", nil))
	}

	quantity := report.CumQty.Sub(regulator.filled)
	cost := report.CumCost.Sub(regulator.cost)
	fee := report.FeeUsdEquiv.Sub(regulator.fee)

	if quantity.Sign() < 0 || cost.Sign() < 0 || fee.Sign() < 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "position: cumulative execution regressed", nil))
	}

	if regulator.Pending.Type == "buy" {
		regulator.Quantity = regulator.Quantity.Add(quantity)
		regulator.Basis = regulator.Basis.Add(cost)
		regulator.EntryFee = regulator.EntryFee.Add(fee)
	}

	if regulator.Pending.Type == "sell" {
		if quantity.Cmp(regulator.Quantity) > 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "position: sale exceeds held inventory", nil))
		}

		basis, entryFee := decimal.NewFromInt64(0), decimal.NewFromInt64(0)

		if quantity.Sign() > 0 {
			basis = regulator.Basis.Mul(quantity).Div(regulator.Quantity)
			entryFee = regulator.EntryFee.Mul(quantity).Div(regulator.Quantity)
		}

		regulator.Quantity = regulator.Quantity.Sub(quantity)
		regulator.Basis = regulator.Basis.Sub(basis)
		regulator.EntryFee = regulator.EntryFee.Sub(entryFee)
		regulator.Realized = regulator.Realized.Add(cost.Sub(fee).Sub(basis).Sub(entryFee))
	}

	regulator.filled, regulator.cost, regulator.fee = report.CumQty, report.CumCost, report.FeeUsdEquiv
	return nil
}
