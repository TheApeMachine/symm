package replay

import (
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
)

func TestHubOpenAndSubscribe(t *testing.T) {
	Convey("Given a recorded capture", t, func() {
		path := filepath.Join(t.TempDir(), "capture.jsonl")
		recorder, err := OpenRecorder(path)

		So(err, ShouldBeNil)

		payload := []byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/EUR"}]}`)
		So(WriteWS(public.TickerChannel, DirectionIn, payload), ShouldBeNil)
		So(recorder.Close(), ShouldBeNil)

		hub, err := Open(path)

		So(err, ShouldBeNil)

		inbound := hub.SubscribeWS(public.TickerChannel)
		frame, ok := <-inbound

		Convey("It should replay websocket frames in order", func() {
			So(ok, ShouldBeTrue)
			So(string(frame), ShouldEqual, string(payload))
		})

		select {
		case _, open := <-hub.Done():
			So(open, ShouldBeFalse)
		case <-time.After(2 * time.Second):
			So("done timeout", ShouldBeBlank)
		}
	})
}

func TestHubMeta(t *testing.T) {
	Convey("Given recorded metadata", t, func() {
		path := filepath.Join(t.TempDir(), "meta.jsonl")
		_, err := OpenRecorder(path)

		So(err, ShouldBeNil)
		So(WriteMeta("symbols", map[string]any{"symbols": []string{"BTC/EUR"}}), ShouldBeNil)

		hub, err := Open(path)

		So(err, ShouldBeNil)

		meta, ok := hub.Meta("symbols")

		Convey("It should index static metadata", func() {
			So(ok, ShouldBeTrue)
			So(string(meta), ShouldContainSubstring, "BTC/EUR")
		})
	})
}
