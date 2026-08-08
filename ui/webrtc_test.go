package ui

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	. "github.com/smartystreets/goconvey/convey"
)

const fluidTestTimeout = 5 * time.Second

func TestFluidRTCAnswer(t *testing.T) {
	Convey("Given a browser peer offering the direct fluid channels", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		publications := make(chan []byte, 2)
		transport := NewFluidRTC(ctx)
		go transport.Run(publications)
		defer func() { So(transport.Close(), ShouldBeNil) }()

		client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
		So(err, ShouldBeNil)
		defer func() { So(client.Close(), ShouldBeNil) }()

		fieldsMessages := make(chan []byte, 8)
		particlesMessages := make(chan []byte, 8)
		opened := make(chan struct{}, 2)
		createFluidTestChannel(client, fluidFieldsChannel, fieldsMessages, opened)
		createFluidTestChannel(client, fluidParticlesChannel, particlesMessages, opened)
		offer, err := client.CreateOffer(nil)
		So(err, ShouldBeNil)
		gathered := webrtc.GatheringCompletePromise(client)
		So(client.SetLocalDescription(offer), ShouldBeNil)
		<-gathered
		answer, err := transport.Answer(*client.LocalDescription())
		So(err, ShouldBeNil)
		So(client.SetRemoteDescription(answer), ShouldBeNil)
		waitForFluidChannels(opened)
		waitForFluidPeerChannels(transport)

		fields := append([]byte(`{"fields":{"Grid":{"x":64,"y":64,"z":64},"Density":"`),
			bytes.Repeat([]byte{'d'}, fluidSegmentSize)...)
		fields = append(fields, []byte(`"}}`)...)
		particles := []byte(`{"particles":[{"Mass":1,"Heat":2}]}`)
		publications <- fields
		publications <- particles

		Convey("It transmits each unmodified JSON value on its named channel", func() {
			So(readFluidTestRecord(fieldsMessages), ShouldResemble, fields)
			So(readFluidTestRecord(particlesMessages), ShouldResemble, particles)
		})
	})
}

func BenchmarkFluidPayloadChannel(b *testing.B) {
	payload := []byte(`{"fields":{"Grid":{"x":64,"y":64,"z":64}}}`)
	b.ReportAllocs()

	for range b.N {
		_, err := fluidPayloadChannel(payload)

		if err != nil {
			b.Fatal(err)
		}
	}
}

func createFluidTestChannel(
	client *webrtc.PeerConnection,
	label string,
	messages chan<- []byte,
	opened chan<- struct{},
) {
	dataChannel, err := client.CreateDataChannel(label, nil)
	So(err, ShouldBeNil)
	dataChannel.OnOpen(func() { opened <- struct{}{} })
	dataChannel.OnMessage(func(message webrtc.DataChannelMessage) {
		messages <- bytes.Clone(message.Data)
	})
}

func waitForFluidChannels(opened <-chan struct{}) {
	for range 2 {
		select {
		case <-opened:
		case <-time.After(fluidTestTimeout):
			So("fluid data channel did not open", ShouldBeEmpty)
		}
	}
}

func waitForFluidPeerChannels(transport *FluidRTC) {
	deadline := time.Now().Add(fluidTestTimeout)

	for time.Now().Before(deadline) {
		transport.mutex.RLock()
		ready := false

		for _, peer := range transport.peers {
			peer.mutex.RLock()
			ready = len(peer.channels) == 2
			peer.mutex.RUnlock()
		}

		transport.mutex.RUnlock()

		if ready {
			return
		}

		time.Sleep(time.Millisecond)
	}

	So("server fluid data channels did not open", ShouldBeEmpty)
}

func readFluidTestRecord(messages <-chan []byte) []byte {
	header := readFluidTestMessage(messages)
	So(header, ShouldHaveLength, fluidRecordHeaderSize)
	So(header[:4], ShouldResemble, fluidRecordMagic[:])
	expected := int(binary.LittleEndian.Uint32(header[4:]))
	record := make([]byte, 0, expected)

	for len(record) < expected {
		record = append(record, readFluidTestMessage(messages)...)
	}

	So(len(record), ShouldEqual, expected)
	return record
}

func readFluidTestMessage(messages <-chan []byte) []byte {
	select {
	case message := <-messages:
		return message
	case <-time.After(fluidTestTimeout):
		So(fmt.Errorf("timed out waiting for fluid message"), ShouldBeNil)
		return nil
	}
}
