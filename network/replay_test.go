package network_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestReplayer(t *testing.T) {
	Convey("Given a Replayer instance", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		replayer := network.NewReplayer(ctx)
		replayer.Reset()

		t1 := int64(1000000000)
		t2 := int64(2000000000)
		t3 := int64(3000000000)
		t4 := int64(4000000000)

		meas1 := data.NewMeasurement(1, "BTC/USD", "trades", time.Unix(0, t1), time.Unix(0, t1))
		meas1.Timestamp = t1
		meas1.Finalize()

		meas2 := data.NewMeasurement(2, "snapshot", "instrument", time.Unix(0, t2), time.Unix(0, t2))
		meas2.Timestamp = t2
		meas2.Finalize()

		meas3 := data.NewMeasurement(3, "BTC/USD", "executions", time.Unix(0, t3), time.Unix(0, t3))
		meas3.Timestamp = t3
		meas3.Finalize()

		meas4 := data.NewMeasurement(4, "balance", "balances", time.Unix(0, t4), time.Unix(0, t4))
		meas4.Timestamp = t4
		meas4.Finalize()

		Convey("When measurements are loaded out of chronological order", func() {
			// Load out-of-order: meas3, meas1, meas4, meas2
			replayer.Load(meas3, meas1, meas4, meas2)

			Convey("Read without keys yields all measurements in strict chronological order", func() {
				buf1, err1 := replayer.Read()
				So(err1, ShouldBeNil)
				r1 := &data.Measurement{}
				So(r1.UnmarshalBinary(buf1), ShouldBeNil)
				So(r1.Timestamp, ShouldEqual, t1)
				So(r1.Source, ShouldEqual, "trades")
				So(time.Now().UnixNano(), ShouldEqual, t1)

				buf2, err2 := replayer.Read()
				So(err2, ShouldBeNil)
				r2 := &data.Measurement{}
				So(r2.UnmarshalBinary(buf2), ShouldBeNil)
				So(r2.Timestamp, ShouldEqual, t2)
				So(r2.Source, ShouldEqual, "instrument")
				So(time.Now().UnixNano(), ShouldEqual, t2)

				buf3, err3 := replayer.Read()
				So(err3, ShouldBeNil)
				r3 := &data.Measurement{}
				So(r3.UnmarshalBinary(buf3), ShouldBeNil)
				So(r3.Timestamp, ShouldEqual, t3)
				So(r3.Source, ShouldEqual, "executions")
				So(time.Now().UnixNano(), ShouldEqual, t3)

				buf4, err4 := replayer.Read()
				So(err4, ShouldBeNil)
				r4 := &data.Measurement{}
				So(r4.UnmarshalBinary(buf4), ShouldBeNil)
				So(r4.Timestamp, ShouldEqual, t4)
				So(r4.Source, ShouldEqual, "balances")
				So(time.Now().UnixNano(), ShouldEqual, t4)

				_, errEmpty := replayer.Read()
				So(errEmpty, ShouldNotBeNil)
			})

			Convey("Rewind resets all stream queues to the beginning of the tape", func() {
				_, _ = replayer.Read()
				_, _ = replayer.Read()

				replayer.Rewind()

				buf, err := replayer.Read()
				So(err, ShouldBeNil)
				restored := &data.Measurement{}
				So(restored.UnmarshalBinary(buf), ShouldBeNil)
				So(restored.Timestamp, ShouldEqual, t1)
				So(time.Now().UnixNano(), ShouldEqual, t1)
			})

			Convey("Seek jumps playback and clock directly to matching timestamp", func() {
				replayer.Seek(t3)

				buf, err := replayer.Read()
				So(err, ShouldBeNil)
				restored := &data.Measurement{}
				So(restored.UnmarshalBinary(buf), ShouldBeNil)
				So(restored.Timestamp, ShouldEqual, t3)
				So(restored.Source, ShouldEqual, "executions")
				So(time.Now().UnixNano(), ShouldEqual, t3)
			})

			Convey("FastForward advances the clock and playback head by duration", func() {
				replayer.Seek(t1)
				replayer.FastForward(1 * time.Second) // t1 (1s) + 1s = 2s (t2)

				buf, err := replayer.Read()
				So(err, ShouldBeNil)
				restored := &data.Measurement{}
				So(restored.UnmarshalBinary(buf), ShouldBeNil)
				So(restored.Timestamp, ShouldEqual, t2)
				So(restored.Source, ShouldEqual, "instrument")
			})

			Convey("Stream-specific Read routes by channel without crosstalk", func() {
				replayer.Rewind()

				instBuf, instErr := replayer.Read("instrument")
				So(instErr, ShouldBeNil)
				instMeas := &data.Measurement{}
				So(instMeas.UnmarshalBinary(instBuf), ShouldBeNil)
				So(instMeas.Source, ShouldEqual, "instrument")

				execBuf, execErr := replayer.Read("executions")
				So(execErr, ShouldBeNil)
				execMeas := &data.Measurement{}
				So(execMeas.UnmarshalBinary(execBuf), ShouldBeNil)
				So(execMeas.Source, ShouldEqual, "executions")
			})
		})
	})
}
