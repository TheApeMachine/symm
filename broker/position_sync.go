package broker

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
)

/*
SnapshotHello copies open-book fields under mu for Tier.Hello / UI. Shared
decimal/stoploss pointers are not returned — callers get detached values.
*/
func (position *Position) SnapshotHello() *Position {
	if position == nil {
		return nil
	}

	position.mu.Lock()
	defer position.mu.Unlock()

	if len(position.Fills) == 0 || position.ClosedAt != nil || position.ID == "" {
		return nil
	}

	out := &Position{
		ID:        position.ID,
		Symbol:    position.Symbol,
		TierKey:   position.TierKey,
		OpenedAt:  position.OpenedAt,
		EntryPath: position.EntryPath,
		Qty:       cloneDecimal(position.Qty),
		Entry:     cloneDecimal(position.Entry),
		Mark:      cloneDecimal(position.Mark),
		PnL:       cloneDecimal(position.PnL),
		PnLPct:    cloneDecimal(position.PnLPct),
	}

	if position.ClosedAt != nil {
		closed := *position.ClosedAt
		out.ClosedAt = &closed
	}

	if position.Stoploss != nil {
		// Scalar fields only — Deltas/Pushes share backing arrays with the live stop.
		out.Stoploss = &Stoploss{
			State:           position.Stoploss.State,
			Regime:          position.Stoploss.Regime,
			InitialPnLPct:   position.Stoploss.InitialPnLPct,
			CurrentPnLPct:   position.Stoploss.CurrentPnLPct,
			PeakPnLPct:      position.Stoploss.PeakPnLPct,
			ArmPnlPct:       position.Stoploss.ArmPnlPct,
			TrailingFloor:   position.Stoploss.TrailingFloor,
			TypicalPullback: position.Stoploss.TypicalPullback,
			Confirms:        position.Stoploss.Confirms,
			ExhaustScore:    position.Stoploss.ExhaustScore,
			TranchesExited:  position.Stoploss.TranchesExited,
			PendingTranches: position.Stoploss.PendingTranches,
			NextTargetPct:   position.Stoploss.NextTargetPct,
		}
	}

	return out
}

/*
AppendHoldPath records a matched path under mu — Model.Step must not append
HoldPaths while Hello/Save race the slice header.
*/
func (position *Position) AppendHoldPath(path string) {
	if position == nil || path == "" {
		return
	}

	position.mu.Lock()
	position.HoldPaths = append(position.HoldPaths, path)
	position.mu.Unlock()
}

/*
SetExitPath stamps exit attribution under mu.
*/
func (position *Position) SetExitPath(path string) {
	if position == nil {
		return
	}

	position.mu.Lock()
	position.ExitPath = path
	position.mu.Unlock()
}

/*
SetEntryDesk stores deskgate entry features under mu.
*/
func (position *Position) SetEntryDesk(features map[string]float64) {
	if position == nil {
		return
	}

	position.mu.Lock()
	position.EntryDesk = cloneFloatMap(features)
	position.mu.Unlock()
}

/*
CreditView is a detached learning snapshot. TakeCreditView marks TreeApplied so
creditExit cannot double-train under concurrent Model ticks.
*/
type CreditView struct {
	EntryPath string
	ExitPath  string
	HoldPaths []string
	EntryDesk map[string]float64
	PnL       *decimal.Decimal
	PnLPct    *decimal.Decimal
	OpenedAt  time.Time
	ClosedAt  *time.Time
	PeakPct   float64
	ArmPct    float64
	HasStop   bool
}

/*
creditViewLocked copies learning fields. Caller holds position.mu.
*/
func (position *Position) creditViewLocked() CreditView {
	view := CreditView{
		EntryPath: position.EntryPath,
		ExitPath:  position.ExitPath,
		HoldPaths: append([]string(nil), position.HoldPaths...),
		EntryDesk: cloneFloatMap(position.EntryDesk),
		PnL:       cloneDecimal(position.PnL),
		PnLPct:    cloneDecimal(position.PnLPct),
		OpenedAt:  position.OpenedAt,
	}

	if position.ClosedAt != nil {
		closed := *position.ClosedAt
		view.ClosedAt = &closed
	}

	if position.Stoploss != nil {
		view.HasStop = true
		view.PeakPct = position.Stoploss.PeakPnLPct
		view.ArmPct = position.Stoploss.ArmPnlPct
	}

	return view
}

/*
SnapshotCreditView copies path/PnL fields under mu without claiming TreeApplied.
*/
func (position *Position) SnapshotCreditView() CreditView {
	if position == nil {
		return CreditView{}
	}

	position.mu.Lock()
	defer position.mu.Unlock()
	return position.creditViewLocked()
}

/*
TakeCreditView copies path/PnL fields under mu and claims TreeApplied.
*/
func (position *Position) TakeCreditView() (CreditView, bool) {
	if position == nil {
		return CreditView{}, false
	}

	position.mu.Lock()
	defer position.mu.Unlock()

	if position.TreeApplied {
		return CreditView{}, false
	}

	if position.ExitPath == "" && len(position.HoldPaths) > 1 {
		position.ExitPath = strings.Join(
			position.HoldPaths[len(position.HoldPaths)-2:], "/",
		)
	}

	view := position.creditViewLocked()
	position.TreeApplied = true
	return view, true
}

/*
validateLocked is Validate for callers that already hold position.mu.
*/
func (position *Position) validateLocked() error {
	return errnie.Require(map[string]any{
		"id":                   position.ID,
		"symbol":               position.Symbol,
		"tier_key":             position.TierKey,
		"qty":                  position.Qty,
		"entry":                position.Entry,
		"entry_order":          position.EntryOrder,
		"entry_venue_order_id": position.EntryVenueOrderID,
		"fills":                position.Fills,
		"price":                position.price,
		"on_settled":           position.onSettled,
	})
}

func cloneDecimal(value *decimal.Decimal) *decimal.Decimal {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneFloatMap(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]float64, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
