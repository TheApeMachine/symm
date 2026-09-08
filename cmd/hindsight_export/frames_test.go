package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/store"
	"gocloud.dev/blob/memblob"
)

func TestExportFrames(t *testing.T) {
	Convey("Public replay exports preserve exact decimal lexemes and exclude private facts", t, func() {
		archive := memblob.OpenBucket(nil)
		Reset(func() { So(archive.Close(), ShouldBeNil) })
		payload := []byte(`{"price":12345.67000000000000000001,"qty":0.0000000010000}`)
		for index, kind := range []string{"level3", "ticker", "balances"} {
			identity := hindsight.CaptureIdentity{Run: "run", Sequence: hindsight.CaptureSequence(index + 1), Stream: hindsight.Stream(kind), StreamEpoch: 1, StreamSequence: uint64(index + 1)}
			So(store.Write(context.Background(), archive, identity.Key(), hindsight.RawFrame{Identity: identity, Endpoint: "ws", Kind: kind, Payload: payload, ReceivedAt: time.Unix(1, 0)}), ShouldBeNil)
		}
		var output bytes.Buffer
		So(exportFrames(archive, "run", 3, &output), ShouldBeNil)
		decoder := json.NewDecoder(&output)
		for range 2 {
			var frame hindsight.RawFrame
			So(decoder.Decode(&frame), ShouldBeNil)
			So(frame.Payload, ShouldResemble, payload)
			So(frame.Kind, ShouldNotEqual, "balances")
		}
		So(decoder.More(), ShouldBeFalse)
		So(output.String(), ShouldNotContainSubstring, "balances")
		output.Reset()
		So(exportFrames(archive, "run", 1, &output), ShouldBeNil)
		var frame hindsight.RawFrame
		So(json.NewDecoder(&output).Decode(&frame), ShouldBeNil)
		So(frame.Identity.Sequence, ShouldEqual, 1)
		So(frame.Payload, ShouldResemble, payload)
	})
}
