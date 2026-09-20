package execution

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
)

type Fill struct {
	ID            string           `json:"id"`
	OrderID       string           `json:"orderId"`
	ClientOrderID string           `json:"clientOrderId"`
	Side          string           `json:"side"` // "buy" or "sell"
	CumQty        *decimal.Decimal `json:"cumQty"`
	CumCost       *decimal.Decimal `json:"cumCost"`
	Fee           *decimal.Decimal `json:"fee"`
	Status        string           `json:"status"` // "open", "filled", "canceled", "rejected"
}

type PositionState struct {
	Symbol   string           `json:"symbol"`
	Quantity *decimal.Decimal `json:"quantity"`
	Basis    *decimal.Decimal `json:"basis"`
	EntryFee *decimal.Decimal `json:"entryFee"`
	Realized *decimal.Decimal `json:"realized"`
}

type RegulatorServer struct {
	quantity *decimal.Decimal
	basis    *decimal.Decimal
	entryFee *decimal.Decimal
	realized *decimal.Decimal

	filled *decimal.Decimal
	cost   *decimal.Decimal
	fee    *decimal.Decimal

	lastOrderID string

	Downstream func(context.Context, *PositionState) error
}

func NewRegulator() *RegulatorServer {
	return &RegulatorServer{
		quantity: decimal.NewFromInt64(0),
		basis:    decimal.NewFromInt64(0),
		entryFee: decimal.NewFromInt64(0),
		realized: decimal.NewFromInt64(0),
		filled:   decimal.NewFromInt64(0),
		cost:     decimal.NewFromInt64(0),
		fee:      decimal.NewFromInt64(0),
	}
}

func (s *RegulatorServer) Write(ctx context.Context, call Regulator_write) error {
	args, err := call.Args().Regulator()
	if err != nil {
		return err
	}

	sym, err := args.Symbol()
	if err != nil {
		return err
	}

	payloadPtr, err := args.Payload()
	if err != nil {
		return err
	}

	var report *Fill
	if payloadPtr.IsValid() {
		data := payloadPtr.Data()
		if len(data) > 0 {
			var parsed Fill
			if err := sonic.Unmarshal(data, &parsed); err == nil {
				report = &parsed
			}
		}
	}

	state := s.evaluate(sym, report)
	if state == nil {
		return nil
	}

	if s.Downstream == nil {
		return nil
	}

	return s.Downstream(ctx, state)
}

func (s *RegulatorServer) Done(ctx context.Context, call Regulator_done) error {
	return nil
}

func (s *RegulatorServer) evaluate(sym string, report *Fill) *PositionState {
	if report == nil {
		return &PositionState{
			Symbol:   sym,
			Quantity: s.quantity,
			Basis:    s.basis,
			EntryFee: s.entryFee,
			Realized: s.realized,
		}
	}

	if report.OrderID != "" && report.OrderID == s.lastOrderID && report.Status == "filled" {
		return &PositionState{
			Symbol:   sym,
			Quantity: s.quantity,
			Basis:    s.basis,
			EntryFee: s.entryFee,
			Realized: s.realized,
		}
	}

	cumQty := report.CumQty
	if cumQty == nil {
		cumQty = decimal.NewFromInt64(0)
	}

	cumCost := report.CumCost
	if cumCost == nil {
		cumCost = decimal.NewFromInt64(0)
	}

	cumFee := report.Fee
	if cumFee == nil {
		cumFee = decimal.NewFromInt64(0)
	}

	deltaQty := cumQty.Sub(s.filled)
	deltaCost := cumCost.Sub(s.cost)
	deltaFee := cumFee.Sub(s.fee)

	if deltaQty.Sign() < 0 || deltaCost.Sign() < 0 || deltaFee.Sign() < 0 {
		errnie.Error(errnie.Err(errnie.Validation, "regulator: cumulative execution regressed", nil))
		return nil
	}

	if report.Side == "buy" && deltaQty.Sign() > 0 {
		s.quantity = s.quantity.Add(deltaQty)
		s.basis = s.basis.Add(deltaCost)
		s.entryFee = s.entryFee.Add(deltaFee)
	}

	if report.Side == "sell" && deltaQty.Sign() > 0 {
		if deltaQty.Cmp(s.quantity) > 0 {
			errnie.Error(errnie.Err(errnie.Validation, "regulator: sale exceeds held inventory", nil))
			return nil
		}

		allocBasis := decimal.NewFromInt64(0)
		allocFee := decimal.NewFromInt64(0)

		if s.quantity.Sign() > 0 {
			allocBasis = s.basis.Mul(deltaQty).Div(s.quantity)
			allocFee = s.entryFee.Mul(deltaQty).Div(s.quantity)
		}

		s.quantity = s.quantity.Sub(deltaQty)
		s.basis = s.basis.Sub(allocBasis)
		s.entryFee = s.entryFee.Sub(allocFee)
		s.realized = s.realized.Add(deltaCost.Sub(deltaFee).Sub(allocBasis).Sub(allocFee))
	}

	s.filled = cumQty
	s.cost = cumCost
	s.fee = cumFee

	switch report.Status {
	case "filled", "canceled", "expired", "rejected":
		s.lastOrderID = report.OrderID
		s.filled = core.ZeroDecimal.Copy()
		s.cost = core.ZeroDecimal.Copy()
		s.fee = core.ZeroDecimal.Copy()
	}

	return &PositionState{
		Symbol:   sym,
		Quantity: s.quantity,
		Basis:    s.basis,
		EntryFee: s.entryFee,
		Realized: s.realized,
	}
}
