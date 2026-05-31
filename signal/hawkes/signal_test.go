package hawkes

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

func tradeBurst(symbol string, base time.Time, count int) []market.TradeUpdate {
	trades := make([]market.TradeUpdate, count)

	for index := range count {
		side := "buy"

		if index%2 == 0 {
			side = "sell"
		}

		trades[index] = market.TradeUpdate{
			Symbol:    symbol,
			Side:      side,
			Price:     100 + float64(index)*0.01,
			Qty:       1.5 + float64(index%5)*0.1,
			Timestamp: base.Add(time.Duration(index) * 100 * time.Millisecond),
		}
	}

	return trades
}

func TestNewSignal(t *testing.T) {
	Convey("Given a qpool", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		Convey("It should expose a measurements broadcast", func() {
			So(signal.broadcasts["measurements"], ShouldNotBeNil)
		})
	})
}

func TestMeasure(t *testing.T) {
	Convey("Given a Hawkes symbol with a clustered buy burst", t, func() {
		symbol := NewHawkesSymbol()
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		ticks := tradeBurst("ALT/EUR", base, 128)
		now := base.Add(128 * 100 * time.Millisecond)

		Convey("When enough arrivals exist to fit", func() {
			measurement, ok := symbol.Measure(ticks, now)

			Convey("It should publish a thermal perspective reading", func() {
				So(ok, ShouldBeTrue)
				So(measurement.Source, ShouldEqual, perspectives.SourceHawkes)
				So(measurement.SNR, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func BenchmarkMeasure(b *testing.B) {
	symbol := NewHawkesSymbol()
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	ticks := tradeBurst("ALT/EUR", base, 128)
	now := base.Add(128 * 100 * time.Millisecond)

	b.ReportAllocs()

	for b.Loop() {
		_, _ = symbol.Measure(ticks, now)
	}
}
