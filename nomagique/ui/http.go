package ui

import (
	"context"
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
	"capnproto.org/go/capnp/v3"
)
type HTTPServerNode types.StreamNode[any, any]

type HTTPServerImpl struct {
	once         sync.Once
	wsClients    sync.Map
	dataChannels sync.Map
	upgrader     websocket.Upgrader
	webrtcAPI    *webrtc.API
	webrtcCfg    webrtc.Configuration
	Downstream   func(context.Context, capnp.Ptr) error
	focusChan    chan string
}

func NewHTTPServer() HTTPServerNode {
	server := &HTTPServerImpl{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(request *http.Request) bool { return true },
		},
		webrtcAPI: webrtc.NewAPI(),
		webrtcCfg: webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{URLs: []string{"stun:stun.l.google.com:19302"}},
			},
			BundlePolicy:  webrtc.BundlePolicyBalanced,
			RTCPMuxPolicy: webrtc.RTCPMuxPolicyRequire,
		},
	}
	
	// Start server immediately on port 8765
	address := ":8765"
	handler := server.buildHTTPHandler()
	httpServer := &http.Server{Addr: address, Handler: handler}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed && !strings.Contains(err.Error(), "address already in use") {
			errnie.Error(errnie.Err(errnie.IO, "[ui] http server failed", err))
		}
	}()

	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			if ch, ok := ctx.Value("focusChan").(chan string); ok {
				server.focusChan = ch
			}
			return nil
		},
		func(next func(context.Context, any) error) {
			server.Downstream = func(c context.Context, ptr capnp.Ptr) error {
				return next(c, ptr)
			}
		},
	)
}

func (s *HTTPServerImpl) Write(ctx context.Context, call HTTPServer_write) error {
	args, err := call.Args().Server()
	if err != nil {
		// fallback to see if it's named something else
		return err
	}
	
	payloadPtr, err := args.Payload()
	if err != nil {
		return err
	}
	
	_, err = args.Addr()
	if err != nil {
		return err
	}
	
	// Server is already started in NewHTTPServer

	if payloadPtr.IsValid() {
		msg := payloadPtr.Message()
		if msg != nil {
			data, err := msg.Marshal()
			if err == nil {
				s.wsClients.Range(func(key, _ any) bool {
					if conn, ok := key.(*websocket.Conn); ok {
						_ = conn.WriteMessage(websocket.BinaryMessage, data)
					}
					return true
				})

				s.dataChannels.Range(func(key, _ any) bool {
					if channel, ok := key.(*webrtc.DataChannel); ok {
						_ = channel.Send(data)
					}
					return true
				})
			}
		}
	}

	if s.Downstream != nil {
		return s.Downstream(ctx, payloadPtr)
	}
	return nil
}

func (s *HTTPServerImpl) Done(ctx context.Context, call HTTPServer_done) error {
	return nil
}

func (s *HTTPServerImpl) buildHTTPHandler() http.Handler {
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

		peerConn, err := s.webrtcAPI.NewPeerConnection(s.webrtcCfg)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}

		peerConn.OnDataChannel(func(channel *webrtc.DataChannel) {
			s.dataChannels.Store(channel, struct{}{})
			channel.OnClose(func() {
				s.dataChannels.Delete(channel)
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
		conn, err := s.upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}

		s.wsClients.Store(conn, struct{}{})

		go func() {
			defer func() {
				s.wsClients.Delete(conn)
				_ = conn.Close()
			}()

			for {
				_, messageBytes, err := conn.ReadMessage()
				if err != nil {
					break
				}
				
				var msg map[string]any
				if err := sonic.Unmarshal(messageBytes, &msg); err == nil {
					if typ, ok := msg["type"].(string); ok && typ == "FOCUS" {
						if symbol, ok := msg["symbol"].(string); ok && symbol != "" {
							if s.focusChan != nil {
								select {
								case s.focusChan <- symbol:
								default:
									// Channel full, drop subscription request
								}
							}
						}
					}
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
