package replay_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/replay"
)

func TestRecorderWriteAndHubPlayback(t *testing.T) {
	convey.Convey("Given a recorded trade frame", t, func() {
		path := filepath.Join(t.TempDir(), "capture.jsonl")
		recorder, err := replay.OpenRecorder(path)

		convey.So(err, convey.ShouldBeNil)

		payload := []byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/EUR","side":"buy","price":1,"qty":1,"ord_type":"market","trade_id":1,"timestamp":"2026-05-31T00:00:00Z"}]}`)
		err = replay.WriteWS(public.TradesChannel, replay.DirectionIn, payload)
		convey.So(err, convey.ShouldBeNil)
		convey.So(recorder.Close(), convey.ShouldBeNil)

		hub, err := replay.Open(path)
		convey.So(err, convey.ShouldBeNil)

		inbound := hub.SubscribeWS(public.TradesChannel)
		frame, ok := <-inbound
		convey.So(ok, convey.ShouldBeTrue)
		convey.So(string(frame), convey.ShouldEqual, string(payload))

		select {
		case _, open := <-hub.Done():
			convey.So(open, convey.ShouldBeFalse)
		case <-time.After(2 * time.Second):
			convey.So("done timeout", convey.ShouldBeBlank)
		}
	})
}

func TestWriteMeta(t *testing.T) {
	convey.Convey("Given a recorder", t, func() {
		path := filepath.Join(t.TempDir(), "meta.jsonl")
		_, err := replay.OpenRecorder(path)
		convey.So(err, convey.ShouldBeNil)

		err = replay.WriteMeta("symbols", map[string]any{"symbols": []string{"BTC/EUR"}})
		convey.So(err, convey.ShouldBeNil)

		hub, err := replay.Open(path)
		convey.So(err, convey.ShouldBeNil)

		meta, ok := hub.Meta("symbols")
		convey.So(ok, convey.ShouldBeTrue)
		convey.So(string(meta), convey.ShouldContainSubstring, "BTC/EUR")
	})
}
