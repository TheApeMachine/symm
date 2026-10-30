package broker

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

func TestHoldDurationDoesNotClose(t *testing.T) {
	Convey("reprice count, a learned bar horizon, and wall-clock age leave the lot open", t, func() {
		stop := NewStopLoss()
		pos := &Position{
			Symbol:          "UNZ/USD",
			PnL:             pct(-1),
			PnLPct:          pct(-1.6),
			OpenedAt:        time.Now().Add(-3 * time.Hour),
			BarsHeld:        33,
			MaxLifetimeBars: 32,
			MaxLifetimeNs:   int64(time.Minute),
		}

		_, first := stop.Evaluate(pos)
		So(first, ShouldBeFalse)

		pos.BarsHeld = 10_000
		trigger, closed := stop.Evaluate(pos)
		So(closed, ShouldBeFalse)
		So(trigger, ShouldEqual, "")
	})
}

func TestStopStateDoesNotArmExpiry(t *testing.T) {
	Convey("the expiry meter stays disarmed however many bars have passed", t, func() {
		stop := NewStopLoss()
		pos := &Position{
			Symbol:          "UNZ/USD",
			PnLPct:          pct(-1.6),
			PeakPnLPct:      pct(-1.6),
			BarsHeld:        33,
			MaxLifetimeBars: 32,
		}

		state := stop.State(pos)
		So(state.Expiry, ShouldEqual, 0)
		So(state.MaxBars, ShouldEqual, 0)
		So(state.BarsHeld, ShouldEqual, 33)
	})
}

func pct(v float64) *decimal.Decimal {
	return decimal.NewFromFloat64(v)
}
