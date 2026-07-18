package types

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

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
CaptureRecovery builds a compact restart payload from live Thesis inventory.
*/
func CaptureRecovery(thesis *Thesis, pending map[string]string, reservations []ReservationWire) *Recovery {
	if thesis == nil {
		return nil
	}

	holdings := map[string]Holding{}

	if thesis.Holdings != nil {
		thesis.Holdings.Range(func(key, value any) bool {
			symbol, ok := key.(string)

			if !ok {
				return true
			}

			switch holding := value.(type) {
			case *Holding:
				if holding != nil {
					holdings[symbol] = *holding
				}
			case Holding:
				holdings[symbol] = holding
			}

			return true
		})
	}

	return &Recovery{
		Holdings:      holdings,
		PendingOrders: pending,
		Reservations:  reservations,
		Tick:          thesis.Tick,
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
