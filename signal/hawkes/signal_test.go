package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

func tradeBurst(symbol string, base time.Time, count int) []krakenmarket.TradeUpdate {
	trades := make([]krakenmarket.TradeUpdate, count)

	for index := range count {
		side := "buy"

		if index%2 == 0 {
			side = "sell"
		}

		trades[index] = krakenmarket.TradeUpdate{
			Symbol:    symbol,
			Side:      side,
			Price:     100 + float64(index)*0.01,
			Qty:       1.5 + float64(index%5)*0.1,
			Timestamp: base.Add(time.Duration(index) * 100 * time.Millisecond),
		}
	}

	return trades
}

func TestHawkesSymbolMeasure(t *testing.T) {
	Convey("Given a Hawkes symbol with a clustered buy burst", t, func() {
		symbol := NewHawkesSymbol()
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		ticks := tradeBurst("ALT/EUR", base, 128)
		now := base.Add(128 * 100 * time.Millisecond)

		Convey("When enough arrivals exist to fit", func() {
			reading, ok := symbol.Measure(ticks, now)

			Convey("It should publish a thermal perspective reading", func() {
				So(ok, ShouldBeTrue)
				So(reading.strength, ShouldBeGreaterThan, 0)
				So(reading.category, ShouldNotEqual, logic.CategoryTypeNone)
			})
		})
	})
}

func TestClassifyHawkesSaturation(t *testing.T) {
	Convey("Given a fit at critical spectral radius", t, func() {
		fit := BivariateFit{
			MuBuy:          1,
			MuSell:         1,
			BuyIntensity:   2,
			SellIntensity:  2,
			SpectralRadius: 0.9,
		}

		category, confidence, _, saturation, _, _ := classifyHawkes(fit, 0.05, false)

		Convey("It should classify saturation", func() {
			So(category, ShouldEqual, logic.CategorySaturation)
			So(confidence, ShouldBeGreaterThan, 0)
			So(saturation, ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	symbolState := NewHawkesSymbol()
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	ticks := tradeBurst("BTC/EUR", base, 128)
	now := base.Add(128 * 100 * time.Millisecond)
	signal := NewSignal("BTC/EUR", logic.NewEntity(logic.EntityTrade), nil, nil, 2.0, 0.5)

	b.ReportAllocs()

	for b.Loop() {
		reading, ok := symbolState.Measure(ticks, now)

		if ok {
			_, _ = signal.publish(reading)
		}
	}
}
