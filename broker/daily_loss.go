package broker

import (
	"math/big"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
DailyLoss tracks realized losses within the current UTC calendar day so the
desk can enforce trading.live.max_daily_loss before admitting new exposure.
Position lifecycle events reach it from the single-writer execution/mark
callback path, so an atomic snapshot swap is sufficient without a mutex.
*/
type DailyLoss struct {
	snapshot atomic.Value
}

type dailyLossSnapshot struct {
	day  int64
	loss decimal.Decimal
}

/*
NewDailyLoss instantiates a DailyLoss with no losses recorded for today.
*/
func NewDailyLoss() *DailyLoss {
	dailyLoss := &DailyLoss{}
	dailyLoss.snapshot.Store(dailyLossSnapshot{
		day:  calendarDay(),
		loss: *decimal.NewFromFloat64(0),
	})

	return dailyLoss
}

func calendarDay() int64 {
	return time.Now().UTC().Truncate(24 * time.Hour).Unix()
}

func (dailyLoss *DailyLoss) current() dailyLossSnapshot {
	if dailyLoss == nil {
		return dailyLossSnapshot{day: calendarDay(), loss: *decimal.NewFromFloat64(0)}
	}

	snapshot := dailyLoss.snapshot.Load().(dailyLossSnapshot)

	if snapshot.day == calendarDay() {
		return snapshot
	}

	return dailyLossSnapshot{day: calendarDay(), loss: *decimal.NewFromFloat64(0)}
}

/*
Record folds a position's realized PnL into the running daily loss. Gains
never reduce a day's tracked loss; only the negative side is accumulated,
so a winning trade cannot mask an earlier losing streak. Crossing into a
new UTC day resets the tracker before the new value is stored.
*/
func (dailyLoss *DailyLoss) Record(realized decimal.Decimal) {
	if dailyLoss == nil || realized.Rat().Sign() >= 0 {
		return
	}

	current := dailyLoss.current()
	scale := int(max(current.loss.GetScale(), realized.GetScale()))
	lossRat := new(big.Rat).Sub(current.loss.Rat(), realized.Rat())
	loss, err := decimal.NewFromString(lossRat.FloatString(scale))

	if err != nil {
		return
	}

	dailyLoss.snapshot.Store(dailyLossSnapshot{day: calendarDay(), loss: *loss})
}

/*
Exceeds reports whether today's accumulated realized loss has reached or
passed the given positive limit. A non-positive limit means no cap is
configured, so it never blocks.
*/
func (dailyLoss *DailyLoss) Exceeds(limit float64) bool {
	if dailyLoss == nil || limit <= 0 {
		return false
	}

	limitRat := new(big.Rat).SetFloat64(limit)

	if limitRat == nil {
		return false
	}

	current := dailyLoss.current()

	return current.loss.Rat().Cmp(limitRat) >= 0
}
