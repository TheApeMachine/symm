package types

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

const RecoveryKey = "recovery"

/*
Recovery is the compact durable restart payload: open holdings, pending order
intents, stop state, and quote reservations. Full Thesis frames stay ephemeral.
*/
type Recovery struct {
	Holdings      map[string]Holding          `json:"holdings"`
	PendingOrders map[string]PendingOrderWire `json:"pendingOrders"`
	Reservations  []ReservationWire           `json:"reservations"`
	Tick          int64                       `json:"tick"`
}

/*
recoveryHolding preserves Holding's Decimal fields for durable checkpoints.
The ordinary Holding JSON projection intentionally serves browser display and
must not define the precision of restart accounting.
*/
type recoveryHolding Holding

/*
MarshalJSON writes durable holdings through their native Decimal encoders while
retaining the existing compact Recovery envelope.
*/
func (recovery Recovery) MarshalJSON() ([]byte, error) {
	if err := recovery.validate(); err != nil {
		return nil, err
	}

	holdings := make(map[string]recoveryHolding, len(recovery.Holdings))

	for symbol, holding := range recovery.Holdings {
		holdings[symbol] = recoveryHolding(holding)
	}

	type recoveryWire struct {
		Holdings      map[string]recoveryHolding  `json:"holdings"`
		PendingOrders map[string]PendingOrderWire `json:"pendingOrders"`
		Reservations  []ReservationWire           `json:"reservations"`
		Tick          int64                       `json:"tick"`
	}

	return json.Marshal(recoveryWire{
		Holdings:      holdings,
		PendingOrders: recovery.PendingOrders,
		Reservations:  recovery.Reservations,
		Tick:          recovery.Tick,
	})
}

/*
PendingOrderWire records an outstanding broker intent for restart reconcile.
*/
type PendingOrderWire struct {
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`
	OrderID       string `json:"orderId,omitempty"`
	Intent        string `json:"intent"`
	ReservationID string `json:"reservationId,omitempty"`
}

/*
ReservationWire is the durable form of a Balance Book claim.
*/
type ReservationWire struct {
	ID     string           `json:"id"`
	Amount *decimal.Decimal `json:"amount"`
	Symbol string           `json:"symbol"`
}

/*
LoadRecovery reads the compact restart payload when present. Missing files are
not an error; malformed payloads stay observable without seeding inventory.
*/
func LoadRecovery(dir string) (*Recovery, error) {
	if dir == "" {
		return nil, nil
	}

	payload, err := os.ReadFile(filepath.Join(dir, RecoveryKey+".json"))

	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, errnie.Error(errnie.Err(errnie.IO, "recovery read failed", err))
	}

	var recovery Recovery

	if err := json.Unmarshal(payload, &recovery); err != nil {
		return nil, errnie.Error(errnie.Err(errnie.UnprocessableContent, "recovery unmarshal failed", err))
	}

	return &recovery, nil
}

/*
SaveRecovery writes the compact restart payload beside the data path.
*/
func SaveRecovery(dir string, recovery *Recovery) error {
	if recovery == nil {
		return nil
	}

	target := filepath.Join(dir, RecoveryKey+".json")
	payload, err := recovery.MarshalJSON()

	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "recovery directory failed", err))
	}

	temporary, err := os.CreateTemp(dir, RecoveryKey+"-*.tmp")

	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "recovery temp failed", err))
	}

	path := temporary.Name()

	if _, err := temporary.Write(payload); err != nil {
		temporary.Close()
		os.Remove(path)
		return errnie.Error(errnie.Err(errnie.IO, "recovery write failed", err))
	}

	if err := temporary.Sync(); err != nil {
		temporary.Close()
		os.Remove(path)
		return errnie.Error(errnie.Err(errnie.IO, "recovery sync failed", err))
	}

	if err := temporary.Close(); err != nil {
		os.Remove(path)
		return errnie.Error(errnie.Err(errnie.IO, "recovery close failed", err))
	}

	if err := os.Rename(path, target); err != nil {
		os.Remove(path)
		return errnie.Error(errnie.Err(errnie.IO, "recovery rename failed", err))
	}

	return nil
}

/*
validate proves every holding can be represented durably without changing a
non-finite value; map identity supplies the symbol when a wire omitted it.
*/
func (recovery Recovery) validate() error {
	for symbol, holding := range recovery.Holdings {
		if holding.Symbol == "" {
			holding.Symbol = symbol
		}

		if _, err := holding.durable(); err != nil {
			return err
		}
	}

	return nil
}

/*
CaptureRecovery builds a compact restart payload from open wallet lots and
rejects non-finite stop state instead of changing its economic meaning.
*/
func CaptureRecovery(
	tick int64,
	open map[string]Holding,
	pending map[string]PendingOrderWire,
	reservations []ReservationWire,
) (*Recovery, error) {
	holdings := map[string]Holding{}

	for symbol, holding := range open {
		if holding.Status == CLOSED || !holding.qtyPositive() {
			continue
		}

		durable, err := holding.durable()

		if err != nil {
			return nil, err
		}

		holdings[symbol] = durable
	}

	return &Recovery{
		Holdings:      holdings,
		PendingOrders: pending,
		Reservations:  reservations,
		Tick:          tick,
	}, nil
}

/*
Apply merges recovered holdings onto a Thesis map for one-shot seed.
*/
func (recovery *Recovery) Apply(holdings *sync.Map) {
	if recovery == nil || holdings == nil {
		return
	}
}

func (holding Holding) qtyPositive() bool {
	return holding.Qty != nil && holding.Qty.Sign() > 0
}

/*
durable copies a holding into a recovery value after proving every persisted
float is finite; the unlocked negative-infinity floor remains a domain sentinel.
*/
func (holding Holding) durable() (Holding, error) {
	out := holding
	out.ctx = nil
	out.cancel = nil

	if out.ReturnPct != nil &&
		(math.IsNaN(*out.ReturnPct) || math.IsInf(*out.ReturnPct, 0)) {
		return Holding{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"recovery: non-finite return for "+holding.Symbol,
			nil,
		))
	}

	if holding.Stoploss != nil {
		if holding.Stoploss.Peak != nil &&
			(math.IsNaN(holding.Stoploss.Peak.Float64()) ||
				math.IsInf(holding.Stoploss.Peak.Float64(), 0)) {
			return Holding{}, errnie.Error(errnie.Err(
				errnie.Validation,
				"recovery: non-finite peak for "+holding.Symbol,
				nil,
			))
		}

		if holding.Stoploss.Floor != nil &&
			(math.IsNaN(holding.Stoploss.Floor.Float64()) ||
				math.IsInf(holding.Stoploss.Floor.Float64(), 0)) {
			return Holding{}, errnie.Error(errnie.Err(
				errnie.Validation,
				"recovery: non-finite stop for "+holding.Symbol,
				nil,
			))
		}
	}

	return out, nil
}

/*
Enrich copies durable entry economics from recovered onto a wallet-backed lot
when the live shell is missing them.
*/
func (holding *Holding) Enrich(recovered Holding) {
	if holding == nil {
		return
	}

	if holding.EntryPrice == nil && recovered.EntryPrice != nil {
		holding.EntryPrice = recovered.EntryPrice.Copy()
	}

	if holding.EntryFee == nil && recovered.EntryFee != nil {
		holding.EntryFee = recovered.EntryFee.Copy()
	}

	if holding.EntryAt == nil && recovered.EntryAt != nil {
		at := *recovered.EntryAt
		holding.EntryAt = &at
	}

	if holding.Asset == "" {
		holding.Asset = recovered.Asset
	}
}

/*
WalletQty is a nil-safe positive-qty check used by Balance reconcile.
*/
func WalletQty(qty *decimal.Decimal) bool {
	return qty != nil && qty.Sign() > 0
}
