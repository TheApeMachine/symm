package trader

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

const checkpointInterval = time.Second

/*
checkpoint enqueues an immutable Recovery DTO so the async flusher never reads
the live Thesis while Planner resets it in place.
*/
func (crypto *Crypto) checkpoint(thesis *types.Thesis) {
	if crypto == nil || thesis == nil || crypto.dataPath == "" {
		return
	}

	crypto.checkpointSlot.Store(crypto.captureRecovery(thesis.Tick))
}

/*
checkpointLoop persists at most once per second from the latest slot.
*/
func (crypto *Crypto) checkpointLoop() {
	ticker := time.NewTicker(checkpointInterval)
	defer ticker.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			crypto.flushCheckpoint()
			return
		case <-ticker.C:
			crypto.flushCheckpoint()
		}
	}
}

func (crypto *Crypto) flushCheckpoint() {
	recovery := crypto.checkpointSlot.Swap(nil)

	if recovery == nil || crypto.dataPath == "" {
		return
	}

	if err := types.SaveRecovery(crypto.dataPath, recovery); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"crypto: recovery checkpoint failed",
			err,
		))
	}
}

/*
captureRecovery builds a durable restart payload from wallet, desk, and books.
*/
func (crypto *Crypto) captureRecovery(tick int64) *types.Recovery {
	pending := map[string]types.PendingOrderWire{}

	if crypto.desk != nil {
		pending = crypto.desk.PendingSymbols()
	}

	reservations := make([]types.ReservationWire, 0)

	if crypto.balance != nil {
		for _, row := range crypto.balance.Snapshots() {
			if row.Amount == nil {
				continue
			}

			amount := row.Amount.Float64()

			if math.IsNaN(amount) || math.IsInf(amount, 0) {
				amount = 0
			}

			reservations = append(reservations, types.ReservationWire{
				ID:     row.ID,
				Amount: amount,
			})
		}
	}

	open := map[string]types.Holding{}

	if crypto.balance != nil {
		for holding := range crypto.balance.Holdings() {
			lot := holding
			crypto.bindCheckpointStop(&lot)
			open[lot.Symbol] = lot
		}
	}

	return types.CaptureRecovery(tick, open, pending, reservations)
}

/*
bindCheckpointStop copies a non-nil desk stop onto the holding for recovery.
*/
func (crypto *Crypto) bindCheckpointStop(lot *types.Holding) {
	if crypto.desk == nil || lot == nil {
		return
	}

	position, ok := crypto.desk.Position(lot.Symbol)

	if !ok {
		return
	}

	stop := position.Stop()

	if stop == nil {
		return
	}

	lot.Stoploss = stop
}
