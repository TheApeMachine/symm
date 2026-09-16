package ui

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

func TestFluidWebRTCRealConnection(t *testing.T) {
	Convey("Given a FluidRTC server with a connected viewer peer", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		server := NewFluidRTC(ctx, "test-server")
		defer server.Close()

		fake := &fakeFluidTransport{}
		channel := testFluidChannel(fake)
		channel.label = types.ManifoldChannel
		channel.start()

		peer := &fluidPeer{
			ctx:           ctx,
			bufferedLimit: 64 * fluidSegmentSize,
			channels: map[string]*fluidChannel{
				types.ManifoldChannel: channel,
			},
		}
		pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
		So(err, ShouldBeNil)
		server.add(pc, peer)

		Convey("server reports that it wants manifold frames", func() {
			So(server.Wants(types.ManifoldChannel), ShouldBeTrue)

			// Publish a full 64x64x64 manifold state (~7.3MB, ~449 chunks)
			dim := 64
			state := &types.ManifoldState{
				Version:     1,
				At:          time.Now(),
				GridX:       dim,
				GridY:       dim,
				GridZ:       dim,
				GridSpacing: 1.0,
				MomRho:      make([]float32, dim*dim*dim*4),
				FieldEnergy: make([]float32, dim*dim*dim),
				WaveReal:    make([]float32, dim*dim*dim),
				WaveImag:    make([]float32, dim*dim*dim),
				Reading: sensorium.Reading{
					CoherenceMag2: 0.5,
				},
			}

			state.MomRho[len(state.MomRho)-1] = 2
			state.FieldEnergy[len(state.FieldEnergy)-1] = 3
			state.WaveReal[len(state.WaveReal)-1] = 4
			state.WaveImag[len(state.WaveImag)-1] = -5

			tee := NewWebRTCTee("fluid-input", 4)
			tee.Transition(runtime.READY)
			defer func() { So(tee.Close(), ShouldBeNil) }()
			measurement := &data.Measurement[float64]{Artifact: state}
			tee.Push(measurement)
			// Reusing the measurement must not replace its already queued artifact.
			err := server.drain(tee)
			So(err, ShouldBeNil)

			// Give the sender goroutine a moment to finish transmitting all chunks
			for i := 0; i < 200; i++ {
				if fake.segmentCount() > 0 && channel.idle() {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}

			totalSegments := fake.segmentCount()
			So(totalSegments, ShouldBeGreaterThan, 400)

			// Verify first chunk header
			firstChunk := fake.segments[0]
			So(len(firstChunk), ShouldBeGreaterThanOrEqualTo, 16)
			So(string(firstChunk[:4]), ShouldEqual, "SFD1")

			totalChunks := binary.LittleEndian.Uint32(firstChunk[12:16])
			So(int(totalChunks), ShouldEqual, totalSegments)

			var payload []byte
			// Verify every chunk has valid header and sequential indices
			for index, chunk := range fake.segments {
				So(string(chunk[:4]), ShouldEqual, "SFD1")
				chunkIndex := binary.LittleEndian.Uint32(chunk[8:12])
				So(int(chunkIndex), ShouldEqual, index)
				payload = append(payload, chunk[fluidChunkHeaderSize:]...)
			}
			message := telemetry.GetRootAsMessage(payload, 0).UnPack()
			frame := message.Frame.Value.(*telemetry.ManifoldFrameT)
			So(frame.GridX, ShouldEqual, dim)
			So(frame.GridY, ShouldEqual, dim)
			So(frame.GridZ, ShouldEqual, dim)
			So(frame.GridSpacing, ShouldEqual, state.GridSpacing)
			So(frame.MomRho, ShouldResemble, state.MomRho)
			So(frame.FieldEnergy, ShouldResemble, state.FieldEnergy)
			So(frame.WaveReal, ShouldResemble, state.WaveReal)
			So(frame.WaveImag, ShouldResemble, state.WaveImag)

		})
	})
}
