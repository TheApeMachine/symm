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
ids, stop state, and quote reservations. Full Thesis frames stay ephemeral.
*/
type Recovery struct {
	Holdings      map[string]Holding `json:"holdings"`
	PendingOrders map[string]string  `json:"pendingOrders"`
	Reservations  []ReservationWire  `json:"reservations"`
	Tick          int64              `json:"tick"`
}

/*
ReservationWire is the durable form of a Balance Book claim.
*/
type ReservationWire struct {
	ID     string  `json:"id"`
	Amount float64 `json:"amount"`
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
	payload, err := json.Marshal(recovery)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "recovery marshal failed", err))
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
CaptureRecovery builds a compact restart payload from open wallet lots.
Non-finite floats are zeroed so encoding/json cannot reject the checkpoint.
*/
func CaptureRecovery(
	tick int64,
	open map[string]Holding,
	pending map[string]string,
	reservations []ReservationWire,
) *Recovery {
	holdings := map[string]Holding{}

	for symbol, holding := range open {
		if holding.Status == CLOSED || !holding.qtyPositive() {
			continue
		}

		holdings[symbol] = holding.durable()
	}

	return &Recovery{
		Holdings:      holdings,
		PendingOrders: pending,
		Reservations:  reservations,
		Tick:          tick,
	}
}

/*
Apply merges recovered holdings onto a Thesis map for one-shot seed.
*/
func (recovery *Recovery) Apply(holdings *sync.Map) {
	if recovery == nil || holdings == nil {
		return
	}

	for symbol, holding := range recovery.Holdings {
		seed := holding
		holdings.Store(symbol, &seed)
	}
}

func (holding Holding) qtyPositive() bool {
	return holding.Qty != nil && holding.Qty.Sign() > 0
}

/*
durable copies a holding into a JSON-safe value for recovery checkpoints.
*/
func (holding Holding) durable() Holding {
	out := holding
	out.ctx = nil
	out.cancel = nil

	if out.Stoploss != nil {
		stop := *out.Stoploss
		stop.ctx = nil
		stop.cancel = nil
		stop.Weight = finiteFloat(stop.Weight)
		stop.LockedFloor = finiteFloat(stop.LockedFloor)
		stop.TrailDistance = finiteFloat(stop.TrailDistance)
		stop.StopReturn = finiteFloat(stop.StopReturn)
		stop.PeakReturn = finiteFloat(stop.PeakReturn)
		stop.MarkReturn = finiteFloat(stop.MarkReturn)
		out.Stoploss = &stop
	}

	if out.ReturnPct != nil {
		value := finiteFloat(*out.ReturnPct)
		out.ReturnPct = &value
	}

	return out
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

	if recovered.Stoploss != nil {
		if holding.Stoploss == nil {
			stop := *recovered.Stoploss
			stop.ctx = nil
			stop.cancel = nil
			holding.Stoploss = &stop
		} else {
			holding.Stoploss.Action = recovered.Stoploss.Action
			holding.Stoploss.Reason = recovered.Stoploss.Reason
			holding.Stoploss.Weight = finiteFloat(recovered.Stoploss.Weight)
			holding.Stoploss.LockedFloor = finiteFloat(recovered.Stoploss.LockedFloor)
			holding.Stoploss.TrailDistance = finiteFloat(recovered.Stoploss.TrailDistance)
			holding.Stoploss.StopReturn = finiteFloat(recovered.Stoploss.StopReturn)
			holding.Stoploss.PeakReturn = finiteFloat(recovered.Stoploss.PeakReturn)
			holding.Stoploss.MarkReturn = finiteFloat(recovered.Stoploss.MarkReturn)
		}
	}

	if holding.Asset == "" {
		holding.Asset = recovered.Asset
	}

	if holding.Stoploss != nil && holding.EntryPrice != nil {
		trail := holding.Stoploss.TrailDistance

		if trail <= 0 && holding.Stoploss.StopReturn < 0 {
			trail = -holding.Stoploss.StopReturn
		}

		holding.Stoploss.Bind(holding.EntryPrice.Float64(), trail)
	}
}

func finiteFloat(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}

	return value
}

/*
WalletQty is a nil-safe positive-qty check used by Balance reconcile.
*/
func WalletQty(qty *decimal.Decimal) bool {
	return qty != nil && qty.Sign() > 0
}
