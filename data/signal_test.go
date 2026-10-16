package data

import (
	"bytes"
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSignals(t *testing.T) {
	Convey("Given a stored tick with signal layers for two symbols", t, func() {
		tick := &Value{Origin: TickOrigin, Role: TickRole, Scope: TickScope, Epoch: 7, Tick: 42}
		tick.FormatPrefix()
		tick.AppendJSON([]byte(`{"bid":"1"}`), "price/BTC/USD")
		AppendSignalVector(tick, "cvd", "BTC/USD", []float64{1.5, -0.25}, "gross_notional", "signed_net_fraction", "window:5m")
		AppendSignalVector(tick, "hawkes", "ETH/USD", []float64{0.75}, "intensity")

		Convey("Its key is the one Prefix derives", func() {
			So(tick.Prefix, ShouldEqual, TickPrefix(7)+"0000000000000000042.bin")
			So(tick.Prefix, ShouldStartWith, "workspace/tick/aggregate/0000000000000000007/")
		})

		Convey("Signals attributes every vector layer after a storage round trip", func() {
			var buf bytes.Buffer
			_, err := io.Copy(&buf, tick)
			So(err, ShouldBeNil)

			stored := New()
			_, err = stored.Write(buf.Bytes())
			So(err, ShouldBeNil)

			signals := make([]Signal, 0)
			for signal, err := range Signals(stored) {
				So(err, ShouldBeNil)
				signals = append(signals, signal)
			}

			So(signals, ShouldHaveLength, 2)
			So(signals[0].Source, ShouldEqual, "cvd")
			So(signals[0].Symbol, ShouldEqual, "BTC/USD")
			So(signals[0].Labels, ShouldResemble, []string{"gross_notional", "signed_net_fraction"})
			So(signals[0].Values, ShouldResemble, []float64{1.5, -0.25})
			So(signals[0].Tags, ShouldResemble, []string{"window:5m"})
			So(signals[1].Source, ShouldEqual, "hawkes")
			So(signals[1].Symbol, ShouldEqual, "ETH/USD")

			value, err := signals[0].Metric("signed_net_fraction")
			So(err, ShouldBeNil)
			So(value, ShouldEqual, -0.25)

			_, err = signals[0].Metric("absent")
			So(err, ShouldNotBeNil)
		})

		Convey("A layer with more values than labels is an error", func() {
			AppendSignalVector(tick, "cvd", "BTC/USD", []float64{1, 2})

			var last error
			for _, err := range Signals(tick) {
				last = err
			}

			So(last, ShouldNotBeNil)
		})
	})
}

func TestKeyEpoch(t *testing.T) {
	Convey("KeyEpoch parses the epoch under a Prefix", t, func() {
		epoch, err := KeyEpoch(TickPrefix(), TickPrefix(1792184828503905000))
		So(err, ShouldBeNil)
		So(epoch, ShouldEqual, uint64(1792184828503905000))

		_, err = KeyEpoch(TickPrefix(), "run/epoch/12/")
		So(err, ShouldNotBeNil)

		_, err = KeyEpoch(TickPrefix(), TickPrefix()+"12/")
		So(err, ShouldNotBeNil)
	})
}
