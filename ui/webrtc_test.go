package ui

import (
	"context"
	"encoding/binary"
	"testing"
	"time"
	"unsafe"

	"github.com/pion/webrtc/v4"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
	"golang.design/x/lockfree/lf"
)

func TestFluidRTCPublish(t *testing.T) {
	Convey("Given a FluidRTC server with a connected viewer peer", t, func() {
		originalRoute := types.Route()
		types.SetRoute("fluid")
		defer types.SetRoute(originalRoute)
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

func TestFluidRTCRun(t *testing.T) {
	Convey("Published owner state crosses an actual WebRTC data channel", t, func() {
		originalRoute := types.Route()
		types.SetRoute("fluid")
		defer types.SetRoute(originalRoute)
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		previous := viper.GetDuration("ui.websocket.learning_interval")
		viper.Set("ui.websocket.learning_interval", time.Millisecond)
		defer viper.Set("ui.websocket.learning_interval", previous)
		server := NewFluidRTC(ctx, "loopback")
		defer func() { So(server.Close(), ShouldBeNil) }()
		settings := webrtc.SettingEngine{}
		settings.SetIncludeLoopbackCandidate(true)
		client, err := webrtc.NewAPI(webrtc.WithSettingEngine(settings)).NewPeerConnection(webrtc.Configuration{})
		So(err, ShouldBeNil)
		defer func() { So(client.Close(), ShouldBeNil) }()
		ordered := false
		retransmits := uint16(0)
		channel, err := client.CreateDataChannel(types.ManifoldChannel, &webrtc.DataChannelInit{Ordered: &ordered, MaxRetransmits: &retransmits})
		So(err, ShouldBeNil)
		received := make(chan []byte, 1)
		channel.OnMessage(func(message webrtc.DataChannelMessage) {
			select {
			case received <- message.Data:
			default:
			}
		})
		offer, err := client.CreateOffer(nil)
		So(err, ShouldBeNil)
		gathered := webrtc.GatheringCompletePromise(client)
		So(client.SetLocalDescription(offer), ShouldBeNil)
		select {
		case <-gathered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		answer, err := server.Answer(*client.LocalDescription())
		So(err, ShouldBeNil)
		So(client.SetRemoteDescription(answer), ShouldBeNil)
		state := &types.ManifoldState{Version: 7, At: time.Unix(100, 0), GridX: 1, GridY: 1, GridZ: 1,
			GridSpacing: 1, MomRho: []float32{1, 2, 3, 4}, FieldEnergy: []float32{5}, WaveReal: []float32{6}, WaveImag: []float32{7}}
		queue := lf.NewQueue[unsafe.Pointer]()
		finished := make(chan error, 1)
		go func() { finished <- server.Run(queue) }()
		defer func() {
			cancel()
			So(<-finished, ShouldBeNil)
		}()
		// Repeated owner notifications also exercise latest-wins publication during negotiation.
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				t.Fatal("No decoded WebRTC frame: ", ctx.Err())
			case <-tick.C:
				var artifact any = state
				queue.Enqueue(unsafe.Pointer(&artifact))
			case packet := <-received:
				So(string(packet[:4]), ShouldEqual, "SFD1")
				So(binary.LittleEndian.Uint32(packet[12:16]), ShouldEqual, 1)
				decoded := telemetry.GetRootAsMessage(packet[fluidChunkHeaderSize:], 0).UnPack()
				frame := decoded.Frame.Value.(*telemetry.ManifoldFrameT)
				So(frame.MomRho, ShouldResemble, state.MomRho)
				So(frame.FieldEnergy, ShouldResemble, state.FieldEnergy)
				return
			}
		}
	})
}
