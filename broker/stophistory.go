package broker

import (
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/store"
)

/*
stopStore is the S3 enqueue surface StopHistory needs — same shape as
store.Client.WriteAtPrefix so tests can record keys without a live bucket.
*/
type stopStore interface {
	WriteAtPrefix(key string, value []byte) (int, error)
}

/*
StopHistory is the temporal stoploss tape: one immutable object per lifecycle
beat (state / regime / trigger / confirms), keyed like workspace ticks so a
trade can be replayed without writing every mark into S3.
*/
type StopHistory struct {
	store   stopStore
	tierKey string
	last    sync.Map // position id → beat signature
}

func NewStopHistory(tierKey string, client *store.Client) *StopHistory {
	history := &StopHistory{tierKey: tierKey}
	if client != nil {
		history.store = client
	}
	return history
}

/*
beat is the lifecycle fingerprint that gates S3 writes. Peak/floor still ride
inside the marshaled body when a beat fires; they do not open a new object alone.
*/
func (stoploss *Stoploss) beat() string {
	if stoploss == nil {
		return ""
	}

	return fmt.Sprintf(
		"%d\x00%s\x00%s\x00%d",
		stoploss.State,
		stoploss.Regime,
		stoploss.TriggerReason,
		stoploss.Confirms,
	)
}

/*
Note parks one already-marshaled Stoploss snapshot under
{tier}/stops/{symbol}/{positionID}/{019d nanos} when beat changes.
*/
func (history *StopHistory) Note(symbol, id, beat string, body []byte) {
	if history == nil || history.store == nil || len(body) == 0 || id == "" || beat == "" {
		return
	}

	if prev, ok := history.last.Load(id); ok && prev.(string) == beat {
		return
	}

	key := fmt.Sprintf(
		"%s/stops/%s/%s/%019d",
		history.tierKey,
		symbol,
		id,
		time.Now().UTC().UnixNano(),
	)

	if _, err := history.store.WriteAtPrefix(key, body); err != nil {
		errnie.Error(err)
		return
	}

	history.last.Store(id, beat)
}
