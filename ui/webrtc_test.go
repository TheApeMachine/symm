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
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

const fluidTestTimeout = 5 * time.Second

func TestFluidRTCAnswer(t *testing.T) {
	Convey("Given a browser peer offering the direct fluid channels", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		publications := transport.NewMapReduce[types.FluidFrame](nil, nil, nil)
		transport := NewFluidRTC(ctx, publications, "test-fluid")
		runDone := make(chan error, 1)
		go func() { runDone <- transport.Run() }()
		defer func() {
			So(transport.Close(), ShouldBeNil)
			So(<-runDone, ShouldBeNil)
		}()

		client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
		So(err, ShouldBeNil)
		defer func() { So(client.Close(), ShouldBeNil) }()

		fieldsMessages := make(chan []byte, 8)
		particlesMessages := make(chan []byte, 8)
		phaseMessages := make(chan []byte, 8)
		opened := make(chan struct{}, 3)
		createFluidTestChannel(client, types.FluidFieldsChannel, fieldsMessages, opened)
		createFluidTestChannel(client, types.FluidParticlesChannel, particlesMessages, opened)
		createFluidTestChannel(client, types.FluidPhaseChannel, phaseMessages, opened)
		offer, err := client.CreateOffer(nil)
		So(err, ShouldBeNil)
		gathered := webrtc.GatheringCompletePromise(client)
		So(client.SetLocalDescription(offer), ShouldBeNil)
		<-gathered
		answer, err := transport.Answer(*client.LocalDescription())
		So(err, ShouldBeNil)
		So(client.SetRemoteDescription(answer), ShouldBeNil)
		waitForFluidChannels(opened, 3)
		waitForFluidPeerChannels(transport)

		fields := append([]byte("SFF1"), bytes.Repeat([]byte{'d'}, fluidSegmentSize)...)
		particles := []byte("SPF1-particles")
		phase := []byte("SYMM-phase")
		publications.Push(types.FluidFrame{Channel: types.FluidFieldsChannel, Payload: fields})
		publications.Push(types.FluidFrame{Channel: types.FluidParticlesChannel, Payload: particles})
		publications.Push(types.FluidFrame{Channel: types.FluidPhaseChannel, Payload: phase})

		Convey("It transmits each unmodified binary record on its named channel", func() {
			So(readFluidTestRecord(fieldsMessages), ShouldResemble, fields)
			So(readFluidTestRecord(particlesMessages), ShouldResemble, particles)
			So(readFluidTestRecord(phaseMessages), ShouldResemble, phase)
		})
	})
}

func TestFluidRTCDiagnosticsPeer(t *testing.T) {
	Convey("Given separate manifold and diagnostics browser peers", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		publications := transport.NewMapReduce[types.FluidFrame](nil, nil, nil)
		transport := NewFluidRTC(ctx, publications, "test-fluid")
		runDone := make(chan error, 1)
		go func() { runDone <- transport.Run() }()
		defer func() {
			So(transport.Close(), ShouldBeNil)
			So(<-runDone, ShouldBeNil)
		}()

		manifold, err := webrtc.NewPeerConnection(webrtc.Configuration{})
		So(err, ShouldBeNil)
		defer func() { So(manifold.Close(), ShouldBeNil) }()
		manifoldOpened := make(chan struct{}, 3)
		createFluidTestChannel(manifold, types.FluidFieldsChannel, make(chan []byte, 1), manifoldOpened)
		createFluidTestChannel(manifold, types.FluidParticlesChannel, make(chan []byte, 1), manifoldOpened)
		createFluidTestChannel(manifold, types.FluidPhaseChannel, make(chan []byte, 1), manifoldOpened)
		answerFluidTestPeer(manifold, transport)
		waitForFluidChannels(manifoldOpened, 3)

		diagnostics, err := webrtc.NewPeerConnection(webrtc.Configuration{})
		So(err, ShouldBeNil)
		defer func() { So(diagnostics.Close(), ShouldBeNil) }()
		diagnosticsMessages := make(chan []byte, 2)
		diagnosticsOpened := make(chan struct{}, 1)
		createFluidTestChannel(
			diagnostics,
			types.DiagnosticsChannel,
			diagnosticsMessages,
			diagnosticsOpened,
		)
		answerFluidTestPeer(diagnostics, transport)
		waitForFluidChannels(diagnosticsOpened, 1)

		payload := []byte("SYMM-diagnostics")
		publications.Push(types.FluidFrame{
			Channel: types.DiagnosticsChannel,
			Payload: payload,
		})

		Convey("It should route only to the peer that owns that channel", func() {
			So(readFluidTestRecord(diagnosticsMessages), ShouldResemble, payload)
			So(transport.Error(), ShouldBeNil)
		})
	})
}

func answerFluidTestPeer(client *webrtc.PeerConnection, transport *FluidRTC) {
	offer, err := client.CreateOffer(nil)
	So(err, ShouldBeNil)
	gathered := webrtc.GatheringCompletePromise(client)
	So(client.SetLocalDescription(offer), ShouldBeNil)
	<-gathered
	answer, err := transport.Answer(*client.LocalDescription())
	So(err, ShouldBeNil)
	So(client.SetRemoteDescription(answer), ShouldBeNil)
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

func waitForFluidChannels(opened <-chan struct{}, count int) {
	for range count {
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
		transport.peersMutex.RLock()
		ready := false

		for _, peer := range transport.peers {
			peer.mutex.RLock()
			ready = len(peer.channels) == 3
			peer.mutex.RUnlock()
		}

		transport.peersMutex.RUnlock()

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
