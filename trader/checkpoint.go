package trader

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

const checkpointInterval = time.Second

/*
checkpoint publishes the newest thesis into a depth-one slot for the async
worker. Trading never blocks on Sync.
*/
func (crypto *Crypto) checkpoint(thesis *types.Thesis) {
	if crypto == nil || thesis == nil || crypto.dataPath == "" {
		return
	}

	crypto.checkpointSlot.Store(thesis)
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
	thesis := crypto.checkpointSlot.Swap(nil)

	if thesis == nil || crypto.dataPath == "" {
		return
	}

	pending := map[string]string{}

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

			if crypto.desk != nil {
				if position, ok := crypto.desk.Position(lot.Symbol); ok {
					if stop := position.Stop(); stop != nil {
						lot.Stoploss = stop
					}
				}
			}

			open[lot.Symbol] = lot
		}
	}

	recovery := types.CaptureRecovery(thesis.Tick, open, pending, reservations)

	if err := types.SaveRecovery(crypto.dataPath, recovery); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"crypto: recovery checkpoint failed",
			err,
		))
	}
}
