package pumpdump

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestObserveTicker(t *testing.T) {
	withPumpdumpConfig(t)

	Convey("Given a pumpdump signal with ticker updates", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		measurements := signal.broadcasts["measurements"].Subscribe("test:pumpdump-ticker", 64)
		base := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

		Convey("When a ticker row has no last price", func() {
			err := signal.observeTicker(market.TickerUpdate{Symbol: "ALT/EUR", Volume: 100})
			So(err, ShouldBeNil)
		})

		Convey("When volume rises across consecutive tickers", func() {
			for index := range 24 {
				ticker := market.TickerUpdate{
					Symbol:    "ALT/EUR",
					Last:      10 + float64(index)*0.05,
					Volume:    float64(1000 + index*50),
					Timestamp: base.Add(time.Duration(index) * time.Second).Format(time.RFC3339Nano),
				}

				err := signal.observeTicker(ticker)

				if err != nil && isWarmup(err) {
					continue
				}

				So(err, ShouldBeNil)
			}

			var measurement types.Measurement
			deadline := time.Now().Add(2 * time.Second)

			for time.Now().Before(deadline) {
				value, waitErr := measurements.Wait(ctx)

				if waitErr != nil {
					break
				}

				if value == nil {
					continue
				}

				reading, ok := value.Value.(types.Measurement)

				if ok && reading.Strength != 0 {
					measurement = reading
					break
				}
			}

			if measurement.Strength == 0 {
				t.Fatalf("timeout waiting for measurement")
			}

			Convey("It should publish a classified reading for the symbol", func() {
				So(measurement.Source, ShouldEqual, types.SourcePumpDump)
				So(measurement.Symbol, ShouldEqual, "ALT/EUR")
				So(measurement.Last, ShouldBeGreaterThan, 0)
				So(measurement.Strength, ShouldNotEqual, 0)
			})
		})
	})
}
