package http

import (
	"bytes"
	"context"
	"io"
	stdhttp "net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/network/webrtc"
	"github.com/theapemachine/symm/nomagique/network/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
	"golang.design/x/lockfree/lf"
)

/*
WorkbenchRunner allows compiler and execution runtimes to register their handlers
with the HTTP server edge.
*/
type WorkbenchRunner interface {
	Compile(rawJSON []byte) (any, error)
	Run(ctx context.Context, rawJSON []byte) (any, error)
	IsPrimitiveUsable(op string) bool
	Primitives() ([]byte, error)
	ListDefinitions() ([]string, error)
	GetDefinition(id string) ([]byte, error)
	SaveDefinition(id string, data []byte) error
}

var (
	workbenchRunnerMu sync.RWMutex
	workbenchRunner   WorkbenchRunner
)

/*
RegisterWorkbenchRunner wires the authoritative compiler/engine runner into the HTTP edge.
*/
func RegisterWorkbenchRunner(runner WorkbenchRunner) {
	workbenchRunnerMu.Lock()
	workbenchRunner = runner
	workbenchRunnerMu.Unlock()
}

/*
HTTPServerServer coordinates HTTP routing, REST API endpoints for the workbench,
and delegates streaming upgrades to the WebSocket and WebRTC protocol servers.
*/
type HTTPServerServer struct {
	*runtime.System
	httpServer   *stdhttp.Server
	wsServer     *websocket.WebSocketServerServer
	webrtcServer *webrtc.WebRTCServerServer
	incoming     *lf.Queue[[]byte]
	out          []byte
}

func NewHTTPServer(ctx context.Context) *HTTPServerServer {
	if ctx == nil {
		ctx = context.Background()
	}

	server := &HTTPServerServer{
		System:       runtime.NewSystem(ctx, "http.server"),
		wsServer:     websocket.NewWebSocketServer(ctx),
		webrtcServer: webrtc.NewWebRTCServer(ctx),
		incoming:     lf.NewQueue[[]byte](),
	}

	handler := server.Handler()
	server.httpServer = &stdhttp.Server{
		Addr:    ":8765",
		Handler: handler,
	}

	go func() {
		err := server.httpServer.ListenAndServe()

		if err != nil && err != stdhttp.ErrServerClosed && !strings.Contains(err.Error(), "address already in use") {
			errnie.Error(errnie.Err(errnie.IO, "[http.server] listen and serve failed", err))
		}
	}()

	go func() {
		<-server.Context().Done()
		_ = server.httpServer.Close()
	}()

	live.Store(server, struct{}{})

	if join, set := joined.Load().(func()); set {
		server.wsServer.OnJoin(join)
	}

	go func() {
		<-server.Context().Done()
		live.Delete(server)
	}()

	server.Transition(runtime.READY)
	return server
}

/*
live holds every running HTTP server, so what the program publishes reaches
whichever server the graph holds without the program knowing its node.
*/
var live sync.Map

/*
OnJoin runs join whenever a client connects to any running server.
*/
func OnJoin(join func()) {
	joined.Store(join)
	live.Range(func(key, _ any) bool {
		key.(*HTTPServerServer).wsServer.OnJoin(join)
		return true
	})
}

var joined atomic.Value

/*
Broadcast sends a binary frame to every client of every running server.
*/
func Broadcast(data []byte) {
	live.Range(func(key, _ any) bool {
		server := key.(*HTTPServerServer)
		server.wsServer.Broadcast(data)
		return true
	})
}

/*
Handler returns the HTTP handler with CORS middleware and all API routes.
*/
func (server *HTTPServerServer) Handler() stdhttp.Handler {
	mux := stdhttp.NewServeMux()

	mux.HandleFunc("GET /workbench/primitives", func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		workbenchRunnerMu.RLock()
		runner := workbenchRunner
		workbenchRunnerMu.RUnlock()

		if runner == nil {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{}`))
			return
		}

		raw, err := runner.Primitives()

		if err != nil {
			stdhttp.Error(writer, err.Error(), stdhttp.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(raw)
	})

	mux.HandleFunc("POST /workbench/compile", func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		body, err := io.ReadAll(request.Body)

		if err != nil {
			stdhttp.Error(writer, err.Error(), stdhttp.StatusBadRequest)
			return
		}

		workbenchRunnerMu.RLock()
		runner := workbenchRunner
		workbenchRunnerMu.RUnlock()

		if runner == nil {
			stdhttp.Error(writer, "workbench runner not registered", stdhttp.StatusServiceUnavailable)
			return
		}

		res, _ := runner.Compile(body)
		writer.Header().Set("Content-Type", "application/json")
		_ = sonic.ConfigDefault.NewEncoder(writer).Encode(res)
	})

	mux.HandleFunc("POST /workbench/run", func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		body, err := io.ReadAll(request.Body)

		if err != nil {
			stdhttp.Error(writer, err.Error(), stdhttp.StatusBadRequest)
			return
		}

		workbenchRunnerMu.RLock()
		runner := workbenchRunner
		workbenchRunnerMu.RUnlock()

		if runner == nil {
			stdhttp.Error(writer, "workbench runner not registered", stdhttp.StatusServiceUnavailable)
			return
		}

		res, _ := runner.Run(request.Context(), body)
		writer.Header().Set("Content-Type", "application/json")
		_ = sonic.ConfigDefault.NewEncoder(writer).Encode(res)
	})

	mux.HandleFunc("GET /workbench/signals", func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		workbenchRunnerMu.RLock()
		runner := workbenchRunner
		workbenchRunnerMu.RUnlock()

		if runner == nil {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`[]`))
			return
		}

		ids, err := runner.ListDefinitions()

		if err != nil {
			stdhttp.Error(writer, err.Error(), stdhttp.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", "application/json")
		_ = sonic.ConfigDefault.NewEncoder(writer).Encode(ids)
	})

	mux.HandleFunc("GET /workbench/signals/{id}", func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		id := request.PathValue("id")
		workbenchRunnerMu.RLock()
		runner := workbenchRunner
		workbenchRunnerMu.RUnlock()

		if runner == nil {
			stdhttp.Error(writer, "workbench runner not registered", stdhttp.StatusServiceUnavailable)
			return
		}

		rawJSON, err := runner.GetDefinition(id)

		if err != nil {
			stdhttp.Error(writer, err.Error(), stdhttp.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(rawJSON)
	})

	mux.HandleFunc("POST /workbench/signals/{id}", func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		id := request.PathValue("id")
		body, err := io.ReadAll(request.Body)

		if err != nil {
			stdhttp.Error(writer, err.Error(), stdhttp.StatusBadRequest)
			return
		}

		workbenchRunnerMu.RLock()
		runner := workbenchRunner
		workbenchRunnerMu.RUnlock()

		if runner == nil {
			stdhttp.Error(writer, "workbench runner not registered", stdhttp.StatusServiceUnavailable)
			return
		}

		if err := runner.SaveDefinition(id, body); err != nil {
			stdhttp.Error(writer, err.Error(), stdhttp.StatusBadRequest)
			return
		}

		writer.WriteHeader(stdhttp.StatusOK)
	})

	mux.HandleFunc("POST /workbench/query", func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"results":[]}`))
	})

	mux.HandleFunc("GET /hindsight/metric-map", func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"baselineCommit":"","metrics":{},"signals":{}}`))
	})

	mux.HandleFunc("GET /hindsight/runs", func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[]`))
	})

	mux.HandleFunc("GET /hindsight/symbols", func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[]`))
	})

	mux.HandleFunc("GET /hindsight/excursions", func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[]`))
	})

	mux.HandleFunc("GET /trades", func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[]`))
	})

	// Mount WebRTC and WebSocket handlers
	mux.HandleFunc("POST /fluid/webrtc/offer", server.webrtcServer.OfferHandler())
	mux.HandleFunc("GET /ws", server.wsServer.UpgradeHandler())
	mux.HandleFunc("GET /hindsight/timeline", server.wsServer.UpgradeHandler())

	return stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")

		if request.Method == stdhttp.MethodOptions {
			writer.WriteHeader(stdhttp.StatusOK)
			return
		}

		mux.ServeHTTP(writer, request)
	})
}

/*
Write broadcasts payload across all active WebSocket and WebRTC connections.
*/
func (server *HTTPServerServer) Write(ctx context.Context, call HTTPServer_write) error {
	data, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"http.server: failed to read data arg",
			err,
		))
	}

	if len(data) > 0 {
		server.wsServer.Broadcast(data)
		server.webrtcServer.Broadcast(data)
		server.out = bytes.Clone(data)
	}

	return nil
}

/*
Done returns the next available message or the last written payload,
along with the current lifecycle status.
*/
func (server *HTTPServerServer) Done(ctx context.Context, call HTTPServer_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"http.server: failed to allocate done results",
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
WebSocketServer returns the underlying WebSocket server component.
*/
func (server *HTTPServerServer) WebSocketServer() *websocket.WebSocketServerServer {
	return server.wsServer
}

/*
WebRTCServer returns the underlying WebRTC server component.
*/
func (server *HTTPServerServer) WebRTCServer() *webrtc.WebRTCServerServer {
	return server.webrtcServer
}

/*
Close stops the HTTP server and associated protocol sub-servers.
*/
func (server *HTTPServerServer) Close() error {
	if server.httpServer != nil {
		_ = server.httpServer.Close()
	}

	_ = server.wsServer.Close()
	_ = server.webrtcServer.Close()
	return server.System.Close()
}
