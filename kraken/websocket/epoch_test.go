package websocket

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/hindsight"
)

/*
recordingCapture is a capture sink that only records the StreamRef it was
handed, so a test can prove the transport's operational epoch is identical
whether or not a sink is attached.
*/
type recordingCapture struct {
	last hindsight.StreamRef
}

func (sink recordingCapture) Capture(
	kind, endpoint string,
	payload []byte,
	receivedAt time.Time,
	ref hindsight.StreamRef,
) (hindsight.CaptureIdentity, error) {
	return hindsight.CaptureIdentity{
		Run:            "test",
		Sequence:       1,
		Stream:         ref.Stream,
		StreamEpoch:    ref.Epoch,
		StreamSequence: ref.Sequence,
	}, nil
}

/*
TestLiveOperationalStreamEpochIsCaptureIndependent proves the transport owns the
reconnect epoch: the same StreamRef sequence is minted whether or not a capture
sink is attached, and a reconnect bumps the epoch with no capture dependency.
*/
func TestLiveOperationalStreamEpochIsCaptureIndependent(t *testing.T) {
	Convey("Given a Live transport with no capture sink", t, func() {
		client := spot.NewWebSocket()
		client.URL = "wss://level3.example"
		live := &Live{client: client}

		Convey("frames mint a monotonic operational epoch/sequence", func() {
			first := live.nextStreamRef("level3")
			second := live.nextStreamRef("level3")

			So(first.Stream, ShouldEqual, hindsight.Stream("wss://level3.example:level3"))
			So(first.Epoch, ShouldEqual, hindsight.StreamEpoch(1))
			So(first.Sequence, ShouldEqual, uint64(1))
			So(second.Sequence, ShouldEqual, uint64(2))
			So(second.Epoch, ShouldEqual, first.Epoch)
		})

		Convey("a reconnect advances the epoch and resets the sequence", func() {
			before := live.nextStreamRef("level3")
			live.reconnectStreams()

			after := live.nextStreamRef("level3")

			So(after.Epoch, ShouldEqual, before.Epoch+1)
			So(after.Sequence, ShouldEqual, uint64(1))
		})
	})

	Convey("The same transport mints identical operational epochs with capture on or off", t, func() {
		withCapture := &Live{
			client:  spot.NewWebSocket(),
			capture: recordingCapture{},
		}
		withCapture.client.URL = "wss://level3.example"

		withoutCapture := &Live{client: spot.NewWebSocket()}
		withoutCapture.client.URL = "wss://level3.example"

		// Drive the identical reconnect + frame sequence through both.
		refWithOn := withCapture.nextStreamRef("level3")
		refWithOff := withoutCapture.nextStreamRef("level3")
		withCapture.reconnectStreams()
		withoutCapture.reconnectStreams()
		afterOn := withCapture.nextStreamRef("level3")
		afterOff := withoutCapture.nextStreamRef("level3")

		So(refWithOn.Stream, ShouldEqual, refWithOff.Stream)
		So(refWithOn.Epoch, ShouldEqual, refWithOff.Epoch)
		So(refWithOn.Sequence, ShouldEqual, refWithOff.Sequence)
		So(afterOn.Epoch, ShouldEqual, afterOff.Epoch)
		So(afterOn.Sequence, ShouldEqual, afterOff.Sequence)
	})
}
