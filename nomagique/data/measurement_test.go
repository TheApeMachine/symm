package data_test

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestMeasurementCapnp(t *testing.T) {
	Convey("Given a Measurement", t, func() {
		normVal := "1.23"
		stdVal := "0.45"

		orig := &data.Measurement[string]{
			ID:         "test-id-123",
			Label:      "BTC/USD",
			Source:     "hawkes",
			SeqIdx:     101,
			Timestamp:  1700000000,
			At:         time.Unix(0, 1700000000123456789),
			SNR:        3.5,
			Maturity:   0.85,
			Metrics: map[string]data.Metric[string]{
				"branching": {
					Raw:          "2.50",
					Normalized:   &normVal,
					Standardized: &stdVal,
				},
			},
			Metadata: map[string]string{
				"action":     "wait",
				"confidence": "0.85",
			},
		}

		Convey("When marshaled to Cap'n Proto bytes", func() {
			encoded, err := orig.MarshalCapnp()
			So(err, ShouldBeNil)
			So(len(encoded), ShouldBeGreaterThan, 0)

			Convey("It round-trips cleanly through UnmarshalMeasurement", func() {
				decoded, err := data.UnmarshalMeasurement(encoded)
				So(err, ShouldBeNil)
				So(decoded, ShouldNotBeNil)

				So(decoded.ID, ShouldEqual, orig.ID)
				So(decoded.Label, ShouldEqual, orig.Label)
				So(decoded.Source, ShouldEqual, orig.Source)
				So(decoded.SeqIdx, ShouldEqual, orig.SeqIdx)
				So(decoded.Timestamp, ShouldEqual, orig.Timestamp)
				So(decoded.At.UnixNano(), ShouldEqual, orig.At.UnixNano())
				So(decoded.SNR, ShouldEqual, orig.SNR)
				So(decoded.Maturity, ShouldEqual, orig.Maturity)

				So(len(decoded.Metrics), ShouldEqual, 1)
				So(decoded.Metadata["action"], ShouldEqual, "wait")
				So(decoded.Metadata["confidence"], ShouldEqual, "0.85")
			})
		})
	})
}
