package ui

import (
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/catalog"
	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/signal"
)

/*
HTTPServer provides a unified HTTP REST, WebSocket, and WebRTC streaming server node.
It serves the frontend endpoints for workbench, hindsight, and real-time execution broadcast.
Pure types.Value closure with internal mux and connection state.
*/
type HTTPServer types.Value[any, any]

func NewHTTPServer(addr types.String) HTTPServer {
	var (
		once         sync.Once
		wsClients    sync.Map
		dataChannels sync.Map
		upgrader     = websocket.Upgrader{
			CheckOrigin: func(request *http.Request) bool { return true },
		}
	)

	webrtcAPI := webrtc.NewAPI()
	webrtcCfg := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
		BundlePolicy:  webrtc.BundlePolicyBalanced,
		RTCPMuxPolicy: webrtc.RTCPMuxPolicyRequire,
	}

	return func(in any) any {
		once.Do(func() {
			address := ":8765"
			if addr != nil {
				if evaluated := addr(in); evaluated != "" {
					address = evaluated
				}
			}

			handler := buildHTTPHandler(&wsClients, &dataChannels, &upgrader, webrtcAPI, webrtcCfg)
			server := &http.Server{Addr: address, Handler: handler}

			go func() {
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed && !strings.Contains(err.Error(), "address already in use") {
					errnie.Error(errnie.Err(errnie.IO, "[ui] http server failed", err))
				}
			}()
		})

		if in != nil {
			var payload []byte
			switch val := in.(type) {
			case []byte:
				payload = val
			case string:
				payload = []byte(val)
			default:
				data, err := sonic.Marshal(val)
				if err == nil {
					payload = data
				}
			}

			if len(payload) > 0 {
				wsClients.Range(func(key, _ any) bool {
					if conn, ok := key.(*websocket.Conn); ok {
						_ = conn.WriteMessage(websocket.TextMessage, payload)
					}
					return true
				})

				dataChannels.Range(func(key, _ any) bool {
					if channel, ok := key.(*webrtc.DataChannel); ok {
						_ = channel.Send(payload)
					}
					return true
				})
			}
		}

		return in
	}
}

func buildHTTPHandler(
	wsClients *sync.Map,
	dataChannels *sync.Map,
	upgrader *websocket.Upgrader,
	webrtcAPI *webrtc.API,
	webrtcCfg webrtc.Configuration,
) http.Handler {
	mux := http.NewServeMux()

	// 1. /workbench/primitives
	mux.HandleFunc("GET /workbench/primitives", func(writer http.ResponseWriter, request *http.Request) {
		primitives, err := catalog.Load()
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", "application/json")
		_ = sonic.ConfigDefault.NewEncoder(writer).Encode(primitives)
	})

	// 2. /workbench/signals
	mux.HandleFunc("GET /workbench/signals", func(writer http.ResponseWriter, request *http.Request) {
		ids, err := signal.ListDefinitions()
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", "application/json")
		_ = sonic.ConfigDefault.NewEncoder(writer).Encode(ids)
	})

	// 3. /workbench/signals/{id}
	mux.HandleFunc("GET /workbench/signals/{id}", func(writer http.ResponseWriter, request *http.Request) {
		id := request.PathValue("id")
		rawJSON, err := signal.GetDefinition(id)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(rawJSON)
	})

	mux.HandleFunc("POST /workbench/signals/{id}", func(writer http.ResponseWriter, request *http.Request) {
		id := request.PathValue("id")
		body, err := io.ReadAll(request.Body)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}

		if err := signal.SaveDefinition(id, body); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}

		writer.WriteHeader(http.StatusOK)
	})

	// 4. /workbench/query
	mux.HandleFunc("POST /workbench/query", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"results":[]}`))
	})

	// 5. /hindsight/metric-map
	mux.HandleFunc("GET /hindsight/metric-map", func(writer http.ResponseWriter, request *http.Request) {
		semantics := signal.Semantics()
		writer.Header().Set("Content-Type", "application/json")
		_ = sonic.ConfigDefault.NewEncoder(writer).Encode(semantics)
	})

	// 6. /hindsight metadata reads
	mux.HandleFunc("GET /hindsight/runs", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[]`))
	})

	mux.HandleFunc("GET /hindsight/symbols", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[]`))
	})

	mux.HandleFunc("GET /hindsight/excursions", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[]`))
	})

	mux.HandleFunc("GET /trades", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[]`))
	})

	// 7. /fluid/webrtc/offer
	mux.HandleFunc("POST /fluid/webrtc/offer", func(writer http.ResponseWriter, request *http.Request) {
		var offer webrtc.SessionDescription
		if err := sonic.ConfigDefault.NewDecoder(request.Body).Decode(&offer); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}

		peerConn, err := webrtcAPI.NewPeerConnection(webrtcCfg)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}

		peerConn.OnDataChannel(func(channel *webrtc.DataChannel) {
			dataChannels.Store(channel, struct{}{})
			channel.OnClose(func() {
				dataChannels.Delete(channel)
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

		gatherComplete := webrtc.GatheringCompletePromise(peerConn)
		if err := peerConn.SetLocalDescription(answer); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		<-gatherComplete

		writer.Header().Set("Content-Type", "application/json")
		_ = sonic.ConfigDefault.NewEncoder(writer).Encode(peerConn.LocalDescription())
	})

	// 8. /ws & /hindsight/timeline websocket upgrade
	wsHandler := func(writer http.ResponseWriter, request *http.Request) {
		conn, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}

		wsClients.Store(conn, struct{}{})

		go func() {
			defer func() {
				wsClients.Delete(conn)
				_ = conn.Close()
			}()

			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					break
				}
			}
		}()
	}

	mux.HandleFunc("GET /ws", wsHandler)
	mux.HandleFunc("GET /hindsight/timeline", wsHandler)

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")

		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusOK)
			return
		}

		mux.ServeHTTP(writer, request)
	})
}
