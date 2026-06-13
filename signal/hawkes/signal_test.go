package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	hkernel "github.com/theapemachine/nomagique/hawkes"
	"github.com/theapemachine/symm/internal/testconfig"
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
			var reading hawkesReading
			var ok bool

			for index := range 4 {
				reading, ok = symbol.Measure(ticks, now.Add(time.Duration(index)*time.Second))
			}

			Convey("It should publish a thermal perspective reading", func() {
				So(ok, ShouldBeTrue)
				So(reading.strength, ShouldBeGreaterThan, 0)
				So(reading.category, ShouldNotEqual, logic.CategoryTypeNone)
			})
		})
	})
}

func TestSignalMeasurePublishesBurst(t *testing.T) {
	Convey("Given a Hawkes signal with a clustered trade burst", t, func() {
		// The signal's measurement ring is sized by regime capacity (window/4).
		// The Hawkes fit needs the full 128-trade burst, so widen the window so
		// the ring can hold it; the default test seed (capacity 4) would drop
		// all but the last few trades and the fit would withhold.
		viper.Set("regime.window", 512)
		viper.Set("regime.baseline.min_obs", 4)

		Reset(func() {
			viper.Set("regime.window", 0)
			viper.Set("regime.baseline.min_obs", 0)
			testconfig.SeedRegimeDefaults()
		})

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		measureAt := base.Add(128 * 100 * time.Millisecond)
		system := &System{}
		signal := NewSignal("ALT/EUR", logic.NewEntity(logic.EntityTrade), system)

		var (
			measurement logic.Measurement
			err         error
		)

		for range 4 {
			for _, trade := range tradeBurst("ALT/EUR", base, 128) {
				update := trade
				signal.Record(&update)
			}

			measurement, err = signal.Measure(nil, measureAt)
		}

		Convey("It should produce a publishable thermal reading", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceHawkes)
			So(measurement.Category, ShouldNotEqual, logic.CategoryTypeNone)
			So(measurement.Publishable(), ShouldBeTrue)
		})
	})
}

func TestClassifyHawkesSaturation(t *testing.T) {
	Convey("Given a fit at critical spectral radius", t, func() {
		fit := hkernel.BivariateFit{
			MuX:            1,
			MuY:            1,
			IntensityX:     2,
			IntensityY:     2,
			SpectralRadius: 0.9,
		}

		gates, gatesReady := hkernel.FitGatesFromHistory(
			[]float64{0.7, 0.75, 0.8, 0.82},
			[]float64{0.05, 0.08, 0.1, 0.12},
		)

		So(gatesReady, ShouldBeTrue)

		category, confidence, _, saturation, _, _ := classifyHawkes(fit, 0.05, false, gates)

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
	signal := NewSignal("BTC/EUR", logic.NewEntity(logic.EntityTrade), nil)

	b.ReportAllocs()

	for b.Loop() {
		reading, ok := symbolState.Measure(ticks, now)

		if ok {
			_, _ = signal.publish(reading, ticks, now)
		}
	}
}
