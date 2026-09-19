package ui

import (
	"net/http"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/pion/webrtc/v4"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
WebRTCServer creates a streaming WebRTC data channel server closure.
It listens for HTTP SDP offers at the configured endpoint, negotiates peer connections,
and broadcasts binary/payload frames across data channels.
No hub application hack, pure Value closure.
*/
type WebRTCServer types.Value[any, any]

func NewWebRTCServer(addr, path types.String) WebRTCServer {
	api := webrtc.NewAPI()
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
		BundlePolicy:  webrtc.BundlePolicyBalanced,
		RTCPMuxPolicy: webrtc.RTCPMuxPolicyRequire,
	}
	var dataChannels sync.Map
	var once sync.Once

	return func(in any) any {
		once.Do(func() {
			a := ":8766"
			if addr != nil {
				if evaluated := addr(in); evaluated != "" {
					a = evaluated
				}
			}
			p := "/webrtc"
			if path != nil {
				if evaluated := path(in); evaluated != "" {
					p = evaluated
				}
			}

			mux := http.NewServeMux()
			mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				var offer webrtc.SessionDescription
				if err := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&offer); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}

				peerConn, err := api.NewPeerConnection(config)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				peerConn.OnDataChannel(func(dc *webrtc.DataChannel) {
					dataChannels.Store(dc, struct{}{})
					dc.OnClose(func() {
						dataChannels.Delete(dc)
					})
				})

				if err := peerConn.SetRemoteDescription(offer); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}

				answer, err := peerConn.CreateAnswer(nil)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				gatherComplete := webrtc.GatheringCompletePromise(peerConn)
				if err := peerConn.SetLocalDescription(answer); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				<-gatherComplete

				w.Header().Set("Content-Type", "application/json")
				_ = sonic.ConfigDefault.NewEncoder(w).Encode(peerConn.LocalDescription())
			})

			server := &http.Server{Addr: a, Handler: mux}
			go func() {
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed && !strings.Contains(err.Error(), "address already in use") {
					errnie.Error(errnie.Err(errnie.IO, "[ui] webrtc server failed", err))
				}
			}()
		})

		if in != nil {
			var payload []byte
			switch v := in.(type) {
			case []byte:
				payload = v
			case string:
				payload = []byte(v)
			default:
				data, err := sonic.Marshal(v)
				if err == nil {
					payload = data
				}
			}
			if len(payload) > 0 {
				dataChannels.Range(func(key, value any) bool {
					if dc, ok := key.(*webrtc.DataChannel); ok && dc.ReadyState() == webrtc.DataChannelStateOpen {
						_ = dc.Send(payload)
					}
					return true
				})
			}
		}

		return in
	}
}
