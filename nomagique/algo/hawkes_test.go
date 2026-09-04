package algo

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestHawkes(t *testing.T) {
	Convey("Given a Hawkes algo configured with Clock and KeyStore", t, func() {
		clock := &temporal.Clock{}
		symbol := "BTC/USD"

		keyStore := store.NewKeyStore(func() string { return symbol })

		hawkes := NewHawkes(HawkesConfig{
			Clock: clock,
			Store: keyStore,
			Key:   func() string { return symbol },
		})

		Convey("stepping a buy arrival produces a valid Measurement", func() {
			at := time.Unix(1000, 0)
			clock.Observe(at)

			hawkes.Step(types.Scalar(1.0))
			measurement := hawkes.Measurement()

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Label, ShouldEqual, "BTC/USD")
			So(measurement.Metrics["event_count"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["event_count:buy"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["event_count:sell"].Raw, ShouldEqual, 0.0)
		})
	})
}
