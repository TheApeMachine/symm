package data

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

func buildTestMeasurement() *Measurement {
	now := time.Now().UTC().Truncate(time.Nanosecond)
	prior := now.Add(-500 * time.Millisecond)

	measurement := NewMeasurement(
		42,
		"BTC/USD",
		"spot:trade",
		now,
		prior,
		&StringEntry{Key: "venue", Value: "kraken"},
		&StringEntry{Key: "source_type", Value: "orderbook"},
	)

	priceExact, err := decimal.NewFromString("91234.56789012")

	if err != nil {
		panic(err)
	}

	priceMetric := NewExactMetric("price", priceExact, UnitPrice, TimescaleInstantaneous)
	priceMetric.snm = 0.0125

	volumeExact, err := decimal.NewFromString("1.50000000")

	if err != nil {
		panic(err)
	}

	volumeMetric := NewExactMetric("volume", volumeExact, UnitVolume, TimescaleInstantaneous)
	volumeMetric.snm = -0.005

	measurement.Write(priceMetric, volumeMetric)

	peer := NewMeasurement(
		42,
		"ETH/USD",
		"spot:trade",
		now,
		prior,
		&StringEntry{Key: "venue", Value: "kraken"},
	)

	ethExact, err := decimal.NewFromString("3456.78000000")

	if err != nil {
		panic(err)
	}

	ethMetric := NewExactMetric("price", ethExact, UnitPrice, TimescaleInstantaneous)
	peer.Write(ethMetric)

	measurement.peers = append(measurement.peers, peer)
	return measurement
}

func TestMeasurementMarshalBinary(t *testing.T) {
	Convey("Given a fully populated, finalized Measurement", t, func() {
		original := buildTestMeasurement()

		Convey("When serialized to binary via MarshalBinary", func() {
			payload, err := original.MarshalBinary()

			So(err, ShouldBeNil)
			So(len(payload), ShouldBeGreaterThan, 0)
			So(string(payload[:len(MeasurementMagic)]), ShouldEqual, MeasurementMagic)

			Convey("Then UnmarshalBinary restores all fields with exact fidelity", func() {
				restored := &Measurement{}
				unmarshalErr := restored.UnmarshalBinary(payload)

				So(unmarshalErr, ShouldBeNil)
				So(restored.ID, ShouldEqual, original.ID)
				So(restored.locked(), ShouldBeTrue)
				So(restored.Epoch, ShouldEqual, original.Epoch)
				So(restored.Label, ShouldEqual, original.Label)
				So(restored.Source, ShouldEqual, original.Source)
				So(restored.SeqIdx, ShouldEqual, original.SeqIdx)
				So(restored.Tick, ShouldEqual, original.Tick)
				So(restored.Timestamp, ShouldEqual, original.Timestamp)
				So(restored.At.Equal(original.At), ShouldBeTrue)
				So(restored.From.Equal(original.From), ShouldBeTrue)
				So(restored.Maturity(), ShouldEqual, original.Maturity())
				So(restored.Samples(), ShouldEqual, original.Samples())
				So(restored.Energy(), ShouldEqual, original.Energy())
				So(restored.Prediction(), ShouldEqual, original.Prediction())

				// Check metrics
				restoredPriceCount := 0

				for entry := range restored.Read("price") {
					restoredPriceCount++
					So(entry.Metric.Raw(), ShouldEqual, original.entry("price").Metric.Raw())
					So(entry.Metric.SNM(), ShouldEqual, original.entry("price").Metric.SNM())
					So(entry.Metric.Unit(), ShouldEqual, UnitPrice)
					So(entry.Metric.Timescale(), ShouldEqual, TimescaleInstantaneous)
					So(entry.Metric.Exact().String(), ShouldEqual, "91234.56789012")
				}

				So(restoredPriceCount, ShouldEqual, 1)

				restoredVolumeCount := 0

				for entry := range restored.Read("volume") {
					restoredVolumeCount++
					So(entry.Metric.Raw(), ShouldEqual, original.entry("volume").Metric.Raw())
					So(entry.Metric.SNM(), ShouldEqual, original.entry("volume").Metric.SNM())
					So(entry.Metric.Unit(), ShouldEqual, UnitVolume)
					So(entry.Metric.Exact().String(), ShouldEqual, "1.50000000")
				}

				So(restoredVolumeCount, ShouldEqual, 1)

				// Check metadata
				metadataMap := make(map[string]string)

				for metaEntry := range restored.Meta() {
					metadataMap[metaEntry.Key] = metaEntry.Value
				}

				So(metadataMap["venue"], ShouldEqual, "kraken")
				So(metadataMap["source_type"], ShouldEqual, "orderbook")

				// Check peers
				restoredPeers := restored.Peers()
				So(len(restoredPeers), ShouldEqual, 1)
				So(restoredPeers[0].Label, ShouldEqual, "ETH/USD")
				So(restoredPeers[0].locked(), ShouldBeTrue)

				for peerEntry := range restoredPeers[0].Read("price") {
					So(peerEntry.Metric.Exact().String(), ShouldEqual, "3456.78000000")
				}
			})
		})
	})
}

func TestMeasurementUnmarshalBinaryInvalid(t *testing.T) {
	Convey("Given Measurement.UnmarshalBinary with invalid data", t, func() {
		measurement := &Measurement{}

		Convey("When given empty data, it returns an explicit error", func() {
			err := measurement.UnmarshalBinary([]byte{})
			So(err, ShouldNotBeNil)
		})

		Convey("When given truncated data, it returns an explicit error", func() {
			err := measurement.UnmarshalBinary([]byte("SYMM\x01truncated"))
			So(err, ShouldNotBeNil)
		})

		Convey("When given wrong magic prefix, it returns an explicit error", func() {
			err := measurement.UnmarshalBinary([]byte("INVALID_DATA"))
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkMarshalBinary(b *testing.B) {
	measurement := buildTestMeasurement()

	b.ReportAllocs()

	for b.Loop() {
		payload, err := measurement.MarshalBinary()

		if err != nil || len(payload) == 0 {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalBinary(b *testing.B) {
	measurement := buildTestMeasurement()
	payload, err := measurement.MarshalBinary()

	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		target := &Measurement{}

		if unmarshalErr := target.UnmarshalBinary(payload); unmarshalErr != nil {
			b.Fatal(unmarshalErr)
		}
	}
}
