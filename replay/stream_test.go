package replay

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
)

func TestStreamRows(t *testing.T) {
	Convey("Given a hub playback", t, func() {
		path := filepath.Join(t.TempDir(), "stream.jsonl")
		recorder, err := OpenRecorder(path)

		So(err, ShouldBeNil)

		payload := []byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/EUR","price":2}]}`)
		So(WriteWS(public.TradesChannel, DirectionIn, payload), ShouldBeNil)
		So(recorder.Close(), ShouldBeNil)

		hub, err := Open(path)

		So(err, ShouldBeNil)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		rows := StreamRows[map[string]any](ctx, hub, public.TradesChannel)
		row, ok := <-rows

		Convey("It should stream decoded rows from the hub", func() {
			So(ok, ShouldBeTrue)
			So((*row)["price"], ShouldEqual, 2)
		})
	})
}

func TestStreamSnapshot(t *testing.T) {
	Convey("Given a snapshot channel", t, func() {
		path := filepath.Join(t.TempDir(), "snapshot.jsonl")
		recorder, err := OpenRecorder(path)

		So(err, ShouldBeNil)

		payload := []byte(`{"channel":"instrument","type":"snapshot","data":{"symbol":"BTC/EUR"}}`)
		So(WriteWS(public.InstrumentsChannel, DirectionIn, payload), ShouldBeNil)
		So(recorder.Close(), ShouldBeNil)

		hub, err := Open(path)

		So(err, ShouldBeNil)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		rows := StreamSnapshot[map[string]any](ctx, hub, public.InstrumentsChannel)
		row, ok := <-rows

		Convey("It should stream object snapshots", func() {
			So(ok, ShouldBeTrue)
			So((*row)["symbol"], ShouldEqual, "BTC/EUR")
		})
	})
}
