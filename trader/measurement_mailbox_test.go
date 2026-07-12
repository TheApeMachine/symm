package trader

import (
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func mailboxMeasurement(value float64) *types.Measurement {
	return &types.Measurement{
		Source: types.SourceToxicity,
		Stream: "level3",
		Symbol: "BTC/USD",
		Metrics: map[string]float64{
			"value": value,
		},
	}
}

func TestMeasurementMailboxStore(t *testing.T) {
	Convey("Given one fixed measurement identity", t, func() {
		mailbox, err := NewMeasurementMailbox(1)
		So(err, ShouldBeNil)

		Convey("When an unread value is replaced", func() {
			So(mailbox.Store(mailboxMeasurement(1)), ShouldBeNil)
			So(mailbox.Store(mailboxMeasurement(2)), ShouldBeNil)

			Convey("It should expose only the latest immutable value and count the replacement", func() {
				measurements := mailbox.Drain()
				So(measurements, ShouldHaveLength, 1)
				So(measurements[0].Metrics["value"], ShouldEqual, 2.0)
				So(mailbox.Superseded(), ShouldEqual, uint64(1))
			})
		})

		Convey("When another identity exceeds fixed capacity", func() {
			So(mailbox.Store(mailboxMeasurement(1)), ShouldBeNil)
			other := mailboxMeasurement(2)
			other.Symbol = "ETH/USD"

			Convey("It should return an explicit capacity error", func() {
				So(mailbox.Store(other), ShouldNotBeNil)
			})
		})
	})
}

func TestMeasurementMailboxDrain(t *testing.T) {
	Convey("Given one producer and one concurrent scanning consumer", t, func() {
		mailbox, err := NewMeasurementMailbox(1)
		So(err, ShouldBeNil)
		So(mailbox.Store(mailboxMeasurement(0)), ShouldBeNil)
		mailbox.Drain()

		const observations = 10_000
		producerDone := make(chan error, 1)
		consumerDone := make(chan float64, 1)

		go func() {
			for observation := 1; observation <= observations; observation++ {
				if err := mailbox.Store(mailboxMeasurement(float64(observation))); err != nil {
					producerDone <- err
					return
				}
			}

			producerDone <- nil
		}()

		go func() {
			latest := 0.0

			for latest < observations {
				for _, measurement := range mailbox.Drain() {
					latest = measurement.Metrics["value"]
				}

				runtime.Gosched()
			}

			consumerDone <- latest
		}()

		Convey("It should publish a complete latest value without sharing mutable slices", func() {
			select {
			case err := <-producerDone:
				So(err, ShouldBeNil)
			case <-time.After(5 * time.Second):
				t.Fatal("measurement producer timed out")
			}

			select {
			case latest := <-consumerDone:
				So(latest, ShouldEqual, float64(observations))
			case <-time.After(5 * time.Second):
				t.Fatal("measurement consumer timed out")
			}
		})
	})
}

func BenchmarkMeasurementMailboxStore(b *testing.B) {
	mailbox, err := NewMeasurementMailbox(1)

	if err != nil {
		b.Fatal(err)
	}

	measurement := mailboxMeasurement(1)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := mailbox.Store(measurement); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMeasurementMailboxDrain(b *testing.B) {
	mailbox, err := NewMeasurementMailbox(1)

	if err != nil {
		b.Fatal(err)
	}

	measurement := mailboxMeasurement(1)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := mailbox.Store(measurement); err != nil {
			b.Fatal(err)
		}

		mailbox.Drain()
	}
}

func BenchmarkMeasurementMailboxDrainTradingTier(b *testing.B) {
	const streamsPerSignal = 2
	tierSize := viper.GetInt("market.universe.trading_tier_size")
	measurements := make([]*types.Measurement, 0, tierSize*streamsPerSignal)

	for index := range tierSize {
		symbol := strconv.Itoa(index)
		level3 := mailboxMeasurement(1)
		level3.Symbol = symbol
		measurements = append(measurements, level3)
		trades := mailboxMeasurement(1)
		trades.Symbol = symbol
		trades.Stream = "trades"
		measurements = append(measurements, trades)
	}

	mailbox, err := NewMeasurementMailbox(len(measurements))

	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for _, measurement := range measurements {
			if err := mailbox.Store(measurement); err != nil {
				b.Fatal(err)
			}
		}

		mailbox.Drain()
	}
}
