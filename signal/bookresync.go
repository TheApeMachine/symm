package signal

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
)

// bookResyncCooldown bounds how often one symbol may be resynced, so a mass
// divergence event (one bus stall drops deltas for the whole universe at once,
// as observed 2026-06-07 18:30:05) becomes one resubscribe wave, not a storm.
const bookResyncCooldown = time.Minute

/*
BookResync heals a diverged local book replica the only way the Kraken v2
protocol allows: unsubscribe + resubscribe the symbol's book channel, which
yields a fresh snapshot. Checksum divergence means a delta was lost — there is
no incremental repair — and before this existed a single dropped frame left the
field degraded for the rest of the session ("still happening").

The request rides the same outbound `kraken:public` broadcast the instrument
manager subscribes through; the snapshot that comes back clears bookDiverged in
every consumer that folds it.
*/
type BookResync struct {
	outbound  *qpool.BroadcastGroup
	lastReq   atomic.Pointer[map[string]time.Time] // copy-on-write: requests are rare (≤1/symbol/min), readers lock-free
	bookDepth int
}

var (
	sharedResyncOnce sync.Once
	sharedResync     *BookResync
)

// SharedBookResync returns the process-wide resync coordinator. Fluid and
// depthflow each owned a private instance before, so one diverged symbol drew
// DUPLICATE unsubscribe/resubscribe waves — same healing, twice the churn.
// One coordinator means one cooldown window per symbol across all consumers.
func SharedBookResync(pool *qpool.Q[any], bookDepth int) *BookResync {
	sharedResyncOnce.Do(func() {
		sharedResync = NewBookResync(pool, bookDepth)
	})

	return sharedResync
}

func NewBookResync(pool *qpool.Q[any], bookDepth int) *BookResync {
	resync := &BookResync{
		// CreateBroadcastGroup is get-or-create: this is the SAME outbound group
		// the websocket Tick drains to the wire, not an orphan.
		outbound:  pool.CreateBroadcastGroup("kraken:public", viper.GetDuration("system.queue.ttl")),
		bookDepth: bookDepth,
	}
	empty := make(map[string]time.Time)
	resync.lastReq.Store(&empty)

	return resync
}

// Request asks the exchange for a fresh book snapshot of symbol, at most once
// per cooldown window per symbol.
func (resync *BookResync) Request(symbol string) {
	if symbol == "" || resync == nil || resync.outbound == nil {
		return
	}

	// Shared across signal goroutines: copy-on-write behind an atomic pointer
	// (the heldSnapshot pattern) — the read is one load, and writes only happen
	// on actual resync requests, which the cooldown caps at one per symbol per
	// minute. CAS failure means another signal just requested this very window;
	// re-checking the fresh map dedupes the race instead of double-sending.
	now := time.Now()

	for {
		current := resync.lastReq.Load()

		if last, seen := (*current)[symbol]; seen && now.Sub(last) < bookResyncCooldown {
			return
		}

		next := make(map[string]time.Time, len(*current)+1)

		for key, value := range *current {
			next[key] = value
		}

		next[symbol] = now

		if resync.lastReq.CompareAndSwap(current, &next) {
			break
		}
	}

	// Unsubscribe first: Kraken treats a duplicate subscribe as an error ack
	// and sends no snapshot. The brief gap is irrelevant — the local book is
	// already wrong.
	resync.outbound.Send(&qpool.QValue[any]{Value: map[string]any{
		"method": "unsubscribe",
		"params": map[string]any{
			"channel": "book",
			"depth":   resync.bookDepth,
			"symbol":  []string{symbol},
		},
	}})

	resync.outbound.Send(&qpool.QValue[any]{Value: map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel":  "book",
			"depth":    resync.bookDepth,
			"symbol":   []string{symbol},
			"snapshot": true,
		},
	}})

	errnie.Debug("book resync requested", "symbol", symbol)
}
