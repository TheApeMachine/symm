package webrtc

import (
	"bytes"
	"context"
	"net/http"
	"sync"

	"github.com/bytedance/sonic"
	pionwebrtc "github.com/pion/webrtc/v4"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"golang.design/x/lockfree/lf"
)

/*
WebRTCServerServer manages WebRTC peer connections, data channels, and ICE
negotiation for low-latency streaming between the market system and browser clients.
*/
type WebRTCServerServer struct {
	*runtime.System
	api          *pionwebrtc.API
	cfg          pionwebrtc.Configuration
	dataChannels sync.Map
	peerConns    sync.Map
	incoming     *lf.Queue[[]byte]
	out          []byte
}

func NewWebRTCServer(ctx context.Context) *WebRTCServerServer {
	server := &WebRTCServerServer{
		System:   runtime.NewSystem(ctx, "webrtc.server"),
		api:      pionwebrtc.NewAPI(),
		incoming: lf.NewQueue[[]byte](),
		cfg: pionwebrtc.Configuration{
			ICEServers: []pionwebrtc.ICEServer{
				{URLs: []string{"stun:stun.l.google.com:19302"}},
			},
			BundlePolicy:  pionwebrtc.BundlePolicyBalanced,
			RTCPMuxPolicy: pionwebrtc.RTCPMuxPolicyRequire,
		},
	}

	server.Transition(runtime.READY)
	return server
}

/*
OfferHandler handles HTTP SDP offer requests, initiates WebRTC peer connections,
registers data channel callbacks, and returns the SDP answer.
*/
func (server *WebRTCServerServer) OfferHandler() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var offer pionwebrtc.SessionDescription

		if err := sonic.ConfigDefault.NewDecoder(request.Body).Decode(&offer); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}

		peerConn, err := server.api.NewPeerConnection(server.cfg)

		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}

		server.peerConns.Store(peerConn, struct{}{})

		peerConn.OnConnectionStateChange(func(state pionwebrtc.PeerConnectionState) {
			if state == pionwebrtc.PeerConnectionStateClosed || state == pionwebrtc.PeerConnectionStateFailed {
				server.peerConns.Delete(peerConn)
			}
		})

		peerConn.OnDataChannel(func(channel *pionwebrtc.DataChannel) {
			server.dataChannels.Store(channel, struct{}{})

			channel.OnMessage(func(msg pionwebrtc.DataChannelMessage) {
				server.incoming.Enqueue(msg.Data)
			})

			channel.OnClose(func() {
				server.dataChannels.Delete(channel)
			})
		})

		if err := peerConn.SetRemoteDescription(offer); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}

		answer, err := peerConn.CreateAnswer(nil)

		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}

		gatherComplete := pionwebrtc.GatheringCompletePromise(peerConn)

		if err := peerConn.SetLocalDescription(answer); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}

		<-gatherComplete

		writer.Header().Set("Content-Type", "application/json")
		_ = sonic.ConfigDefault.NewEncoder(writer).Encode(peerConn.LocalDescription())
	}
}

/*
Write transmits a binary payload across all open WebRTC data channels.
*/
func (server *WebRTCServerServer) Write(ctx context.Context, call WebRTCServer_write) error {
	data, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"webrtc.server: failed to read data arg",
			err,
		))
	}

	if len(data) > 0 {
		server.Broadcast(data)
		server.out = bytes.Clone(data)
	}

	return nil
}

/*
Broadcast sends a message buffer across all connected data channels.
*/
func (server *WebRTCServerServer) Broadcast(data []byte) {
	server.dataChannels.Range(func(key, _ any) bool {
		channel, ok := key.(*pionwebrtc.DataChannel)

		if ok {
			_ = channel.Send(data)
		}

		return true
	})
}

/*
Done returns the next received channel message or the last broadcasted payload,
along with the current lifecycle status.
*/
func (server *WebRTCServerServer) Done(ctx context.Context, call WebRTCServer_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"webrtc.server: failed to allocate done results",
			err,
		))
	}

	results.SetStatus(runtime.Status(server.Status()))

	if server.Status() != runtime.READY {
		return nil
	}

	msg, ok := server.incoming.Dequeue()

	if ok {
		return results.SetOut(msg)
	}

	if len(server.out) > 0 {
		err := results.SetOut(server.out)
		server.out = nil
		return err
	}

	return nil
}

/*
Close terminates all data channels and active peer connections.
*/
func (server *WebRTCServerServer) Close() error {
	server.dataChannels.Range(func(key, _ any) bool {
		channel, ok := key.(*pionwebrtc.DataChannel)

		if ok {
			_ = channel.Close()
		}

		return true
	})

	server.peerConns.Range(func(key, _ any) bool {
		conn, ok := key.(*pionwebrtc.PeerConnection)

		if ok {
			_ = conn.Close()
		}

		return true
	})

	return server.System.Close()
}
