package conditions_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
)

func TestBuilders(t *testing.T) {
	Convey("Given named market conditions", t, func() {
		markets := []struct {
			name   string
			market *tests.Market
		}{
			{"calm", conditions.Calm(4)},
			{"pump", conditions.Pump(4, 2, 1.2, 4)},
			{"drawdown", conditions.Drawdown(4, 0.1, 2)},
			{"reversal", conditions.Reversal(4, 2, 0.05)},
			{"aggression", conditions.Aggression(4, 2, 4)},
			{"decay", conditions.Decay(4, 0, 0.8)},
			{"imbalance", conditions.Imbalance(4, 0, 3, 0.3)},
			{"lag", conditions.Lag(4, 2)},
			{"herd", conditions.Herd(4)},
			{"noise", conditions.Noise(4)},
			{"phantom_drawdown", conditions.PhantomDrawdown(4, 1, 0.015)},
			{"calibrated_lift", conditions.CalibratedLift(4, 1, 1.04)},
		}

		for _, entry := range markets {
			count := 0

			for range entry.market.Frames() {
				count++
			}

			So(count, ShouldBeGreaterThan, 4)
		}

		So(conditions.Subject(), ShouldEqual, "MATIC/USD")
	})
}
