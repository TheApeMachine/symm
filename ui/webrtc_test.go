package ui

import (
	"context"
	"testing"

	"github.com/pion/webrtc/v4"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

func TestFluidRTCAnswer(t *testing.T) {
	Convey("Given a FluidRTC transport instance", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		workspace := runtime.NewWorkspace(ctx)
		defer workspace.Close()

		fluid := NewFluidRTC(ctx, workspace, "fluid")
		So(fluid, ShouldNotBeNil)
		defer fluid.Close()

		Convey("When a client sends an offer", func() {
			clientPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
			So(err, ShouldBeNil)
			defer clientPC.Close()

			dc, err := clientPC.CreateDataChannel(types.ManifoldChannel, &webrtc.DataChannelInit{
				Ordered: new(true),
			})
			So(err, ShouldBeNil)
			So(dc, ShouldNotBeNil)

			offer, err := clientPC.CreateOffer(nil)
			So(err, ShouldBeNil)
			So(clientPC.SetLocalDescription(offer), ShouldBeNil)

			answer, err := fluid.Answer(offer)

			Convey("It should generate a valid answer without canceling the transport context", func() {
				So(err, ShouldBeNil)
				So(answer.SDP, ShouldNotBeEmpty)
				So(fluid.ctx.Err(), ShouldBeNil)
			})
		})

		Convey("When a client peer disconnects or fails", func() {
			clientPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
			So(err, ShouldBeNil)

			dc, err := clientPC.CreateDataChannel(types.ManifoldChannel, &webrtc.DataChannelInit{
				Ordered: new(true),
			})
			So(err, ShouldBeNil)
			So(dc, ShouldNotBeNil)

			offer, err := clientPC.CreateOffer(nil)
			So(err, ShouldBeNil)
			So(clientPC.SetLocalDescription(offer), ShouldBeNil)

			_, err = fluid.Answer(offer)
			So(err, ShouldBeNil)

			// Force client disconnect
			So(clientPC.Close(), ShouldBeNil)

			Convey("A subsequent new browser offer should still succeed", func() {
				newClientPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
				So(err, ShouldBeNil)
				defer newClientPC.Close()

				newDC, err := newClientPC.CreateDataChannel(types.ManifoldChannel, &webrtc.DataChannelInit{
					Ordered: new(true),
				})
				So(err, ShouldBeNil)
				So(newDC, ShouldNotBeNil)

				newOffer, err := newClientPC.CreateOffer(nil)
				So(err, ShouldBeNil)
				So(newClientPC.SetLocalDescription(newOffer), ShouldBeNil)

				newAnswer, err := fluid.Answer(newOffer)
				So(err, ShouldBeNil)
				So(newAnswer.SDP, ShouldNotBeEmpty)
			})
		})
	})
}

func TestFluidRTCPublish(t *testing.T) {
	Convey("Given a FluidRTC transport instance", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		workspace := runtime.NewWorkspace(ctx)
		defer workspace.Close()

		fluid := NewFluidRTC(ctx, workspace, "fluid")
		So(fluid, ShouldNotBeNil)
		defer fluid.Close()

		Convey("Publishing to no active peers should succeed cleanly", func() {
			err := fluid.publish(types.ManifoldChannel, []byte("test"))
			So(err, ShouldBeNil)
		})
	})
}

func TestFluidRTCClose(t *testing.T) {
	Convey("Given an active FluidRTC instance", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		workspace := runtime.NewWorkspace(ctx)
		defer workspace.Close()

		fluid := NewFluidRTC(ctx, workspace, "fluid")
		So(fluid, ShouldNotBeNil)

		Convey("Close should cancel the transport context and release peers", func() {
			So(fluid.Close(), ShouldBeNil)
			So(fluid.ctx.Err(), ShouldNotBeNil)
		})
	})
}
