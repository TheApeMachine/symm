package types

import (
	"encoding/json"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

func TestHoldingMarshalJSON(t *testing.T) {
	Convey("Given an open lot with a bound stop", t, func() {
		pct := 0.005
		stop := NewStoploss(t.Context())
		stop.Bind(100, 0.01)

		holding := Holding{
			Symbol:     "BTC/USD",
			Qty:        decimal.NewFromFloat64(0.01),
			EntryPrice: decimal.NewFromFloat64(100),
			Mark:       decimal.NewFromFloat64(101),
			PnL:        decimal.NewFromFloat64(0.5),
			ReturnPct:  &pct,
			Status:     OPEN,
			Stoploss:   stop,
		}

		payload, err := json.Marshal(holding)

		Convey("It should emit finite JSON with derived stop_price", func() {
			So(err, ShouldBeNil)
			So(json.Valid(payload), ShouldBeTrue)
			So(string(payload), ShouldContainSubstring, `"pnl":0.5`)
			So(string(payload), ShouldContainSubstring, `"stop_price":99`)
			So(string(payload), ShouldContainSubstring, `"lockedFloor":0`)
		})
	})
}

func BenchmarkHoldingMarshalJSON(b *testing.B) {
	pct := 0.005
	stop := NewStoploss(b.Context())
	stop.Bind(100, 0.01)
	holding := Holding{
		Symbol:     "BTC/USD",
		Qty:        decimal.NewFromFloat64(0.01),
		EntryPrice: decimal.NewFromFloat64(100),
		Mark:       decimal.NewFromFloat64(101),
		PnL:        decimal.NewFromFloat64(0.5),
		ReturnPct:  &pct,
		Status:     OPEN,
		Stoploss:   stop,
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := json.Marshal(holding); err != nil {
			b.Fatal(err)
		}
	}
}
