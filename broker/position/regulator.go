package position

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

// Regulator owns inventory and one pending order. Only cumulative venue fills
// change inventory; submission and acknowledgement are not fills.
type Regulator struct {
	mu             sync.RWMutex
	PositionID     string
	Symbol         string
	Quantity       *decimal.Decimal
	Basis          *decimal.Decimal
	ClosedBasis    *decimal.Decimal
	closedEntryFee *decimal.Decimal
	closedFee      *decimal.Decimal
	EntryFee       *decimal.Decimal
	Realized       *decimal.Decimal
	Pending        *kraken.AddOrderRequest
	OrderID        string
	LastOrderID    string
	EntryAt        time.Time
	filled         *decimal.Decimal
	cost           *decimal.Decimal
	fee            *decimal.Decimal
}

func NewRegulator(symbol string) *Regulator {
	return &Regulator{
		PositionID:     uuid.New().String(),
		Symbol:         symbol,
		Quantity:       decimal.NewFromInt64(0).SetScale(decimal.DefaultScale),
		Basis:          decimal.NewFromInt64(0).SetScale(decimal.DefaultScale),
		ClosedBasis:    decimal.NewFromInt64(0).SetScale(decimal.DefaultScale),
		closedEntryFee: decimal.NewFromInt64(0).SetScale(decimal.DefaultScale),
		closedFee:      decimal.NewFromInt64(0).SetScale(decimal.DefaultScale),
		EntryFee:       decimal.NewFromInt64(0).SetScale(decimal.DefaultScale),
		Realized:       decimal.NewFromInt64(0).SetScale(decimal.DefaultScale),
		EntryAt:        time.Now(),
		filled:         decimal.NewFromInt64(0).SetScale(decimal.DefaultScale),
		cost:           decimal.NewFromInt64(0).SetScale(decimal.DefaultScale),
		fee:            decimal.NewFromInt64(0).SetScale(decimal.DefaultScale),
	}
}

func (regulator *Regulator) Status() string {
	if regulator == nil {
		return "closed"
	}

	regulator.mu.RLock()
	defer regulator.mu.RUnlock()

	if regulator.Pending != nil {
		if regulator.Pending.Type == "sell" {
			return "exit_pending"
		}

		return "entry_pending"
	}

	if regulator.Quantity != nil && regulator.Quantity.Sign() > 0 {
		return "open"
	}

	return "closed"
}

func (regulator *Regulator) IsHolding() bool {
	if regulator == nil {
		return false
	}

	regulator.mu.RLock()
	defer regulator.mu.RUnlock()

	return regulator.Quantity != nil && regulator.Quantity.Sign() > 0
}

func (regulator *Regulator) IsClosed() bool {
	if regulator == nil {
		return true
	}

	regulator.mu.RLock()
	defer regulator.mu.RUnlock()

	return regulator.Pending == nil && (regulator.Quantity == nil || regulator.Quantity.Sign() == 0)
}

func (regulator *Regulator) Price() *decimal.Decimal {
	if regulator == nil {
		return nil
	}

	regulator.mu.RLock()
	defer regulator.mu.RUnlock()

	if regulator.Quantity != nil && regulator.Quantity.Sign() > 0 && regulator.Basis != nil {
		return regulator.Basis.Div(regulator.Quantity)
	}

	if regulator.Pending != nil && regulator.Pending.Price != "" {
		price, err := decimal.NewFromString(regulator.Pending.Price)

		if err == nil {
			return price
		}
	}

	return nil
}

func (regulator *Regulator) ClosedCost() *decimal.Decimal {
	if regulator == nil {
		return nil
	}

	regulator.mu.RLock()
	defer regulator.mu.RUnlock()
	return safeAdd(regulator.ClosedBasis, regulator.closedEntryFee)
}

func (regulator *Regulator) ClosedFee() *decimal.Decimal {
	if regulator == nil {
		return nil
	}

	regulator.mu.RLock()
	defer regulator.mu.RUnlock()
	return regulator.closedFee
}

func (regulator *Regulator) Volume() *decimal.Decimal {
	if regulator == nil {
		return nil
	}

	regulator.mu.RLock()
	defer regulator.mu.RUnlock()

	return regulator.Quantity
}

func (regulator *Regulator) Fee() *decimal.Decimal {
	if regulator == nil {
		return nil
	}

	regulator.mu.RLock()
	defer regulator.mu.RUnlock()

	return regulator.EntryFee
}

func (regulator *Regulator) Identifies(orderID, clientOrderID string) bool {
	if regulator == nil {
		return false
	}

	regulator.mu.RLock()
	defer regulator.mu.RUnlock()

	if orderID != "" {
		if orderID == regulator.OrderID || orderID == regulator.LastOrderID {
			return true
		}
	}

	if clientOrderID != "" {
		if clientOrderID == regulator.PositionID {
			return true
		}

		if regulator.Pending != nil && clientOrderID == regulator.Pending.ClOrdId {
			return true
		}
	}

	return false
}

func (regulator *Regulator) SetOrderID(orderID string) {
	if regulator == nil || orderID == "" {
		return
	}

	regulator.mu.Lock()
	defer regulator.mu.Unlock()

	regulator.OrderID = orderID
}

func (regulator *Regulator) CancelPending() {
	if regulator == nil {
		return
	}

	regulator.mu.Lock()
	defer regulator.mu.Unlock()

	regulator.Pending = nil
}

func (regulator *Regulator) Begin(request *kraken.AddOrderRequest) error {
	if regulator == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "position: nil regulator", nil))
	}

	regulator.mu.Lock()
	defer regulator.mu.Unlock()

	if regulator.Pending != nil {
		return errnie.Error(errnie.Err(errnie.Conflict, "position: order already pending", nil))
	}

	if request == nil || request.Pair != regulator.Symbol {
		return errnie.Error(errnie.Err(errnie.Validation, "position: identified order required", nil))
	}

	if request.ClOrdId == "" {
		request.ClOrdId = regulator.PositionID
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
	if regulator == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "position: nil regulator", nil))
	}

	regulator.mu.Lock()
	defer regulator.mu.Unlock()

	if report.OrderID != "" && report.OrderID == regulator.LastOrderID {
		return nil
	}

	matchesOrder := report.OrderID != "" && report.OrderID == regulator.OrderID
	matchesClient := regulator.Pending != nil && (report.ClientOrderID == regulator.Pending.ClOrdId || report.ClientOrderID == regulator.PositionID)

	if !matchesOrder && !matchesClient {
		return errnie.Error(errnie.Err(errnie.Validation, "position: execution does not identify pending order", nil))
	}

	if regulator.OrderID == "" && report.OrderID != "" {
		regulator.OrderID = report.OrderID
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
	if regulator.Pending == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "position: execution without pending order", nil))
	}

	cumCost := report.CumCost

	if cumCost == nil {
		price := report.AvgPrice

		if price == nil || price.Sign() <= 0 {
			price = report.LastPrice
		}

		if price != nil && report.CumQty != nil {
			cumCost = price.Mul(report.CumQty)
		}
	}

	fee := report.FeeUsdEquiv

	if fee == nil {
		if len(report.Fees) > 0 {
			fee = decimal.NewFromFloat64(report.Fees[0].Qty)
		}

		if fee == nil {
			fee = decimal.NewFromInt64(0)
		}
	}

	if cumCost == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "position: cumulative cost required", nil))
	}

	quantity := safeSub(report.CumQty, regulator.filled)
	cost := safeSub(cumCost, regulator.cost)
	feeDelta := safeSub(fee, regulator.fee)

	if quantity.Sign() < 0 || cost.Sign() < 0 || feeDelta.Sign() < 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "position: cumulative execution regressed", nil))
	}

	if regulator.Pending.Type == "buy" {
		regulator.Quantity = safeAdd(regulator.Quantity, quantity)
		regulator.Basis = safeAdd(regulator.Basis, cost)
		regulator.EntryFee = safeAdd(regulator.EntryFee, feeDelta)
	}

	if regulator.Pending.Type == "sell" {
		if quantity.Cmp(regulator.Quantity) > 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "position: sale exceeds held inventory", nil))
		}

		basis := decimal.NewFromInt64(0).SetScale(decimal.DefaultScale)
		entryFee := decimal.NewFromInt64(0).SetScale(decimal.DefaultScale)

		if quantity.Sign() > 0 && regulator.Quantity.Sign() > 0 {
			basis = regulator.Basis.Mul(quantity).Div(regulator.Quantity)
			entryFee = regulator.EntryFee.Mul(quantity).Div(regulator.Quantity)
		}

		regulator.Quantity = safeSub(regulator.Quantity, quantity)
		regulator.Basis = safeSub(regulator.Basis, basis)
		regulator.ClosedBasis = safeAdd(regulator.ClosedBasis, basis)
		regulator.closedEntryFee = safeAdd(regulator.closedEntryFee, entryFee)
		regulator.closedFee = safeAdd(regulator.closedFee, safeAdd(entryFee, feeDelta))
		regulator.EntryFee = safeSub(regulator.EntryFee, entryFee)

		pnl := safeSub(safeSub(safeSub(cost, feeDelta), basis), entryFee)
		regulator.Realized = safeAdd(regulator.Realized, pnl)
	}

	regulator.filled = report.CumQty
	regulator.cost = cumCost
	regulator.fee = fee
	return nil
}

func safeSub(a, b *decimal.Decimal) *decimal.Decimal {
	if a == nil {
		return nil
	}

	if b == nil {
		return a
	}

	scale := max(a.GetScale(), b.GetScale())

	if scale < decimal.DefaultScale {
		scale = decimal.DefaultScale
	}

	return a.SetScale(scale).Sub(b)
}

func safeAdd(a, b *decimal.Decimal) *decimal.Decimal {
	if a == nil {
		return b
	}

	if b == nil {
		return a
	}

	scale := max(a.GetScale(), b.GetScale())

	if scale < decimal.DefaultScale {
		scale = decimal.DefaultScale
	}

	return a.SetScale(scale).Add(b)
}
