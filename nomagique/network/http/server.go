package http

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	stdhttp "net/http"
	"sync"

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
	httpServer *stdhttp.Server
	*websocket.WebSocketServerServer
	webrtcServer *webrtc.WebRTCServerServer
	incoming     *lf.Queue[[]byte]
	out          []byte
	bind         error
	address      string
	listening    bool
	inspection   inspection
}

func NewHTTPServer(ctx context.Context) *HTTPServerServer {
	if ctx == nil {
		ctx = context.Background()
	}

	server := &HTTPServerServer{
		System:                runtime.NewSystem(ctx, "http.server"),
		WebSocketServerServer: websocket.NewWebSocketServer(ctx),
		webrtcServer:          webrtc.NewWebRTCServer(ctx),
		incoming:              lf.NewQueue[[]byte](),
	}

	server.httpServer = &stdhttp.Server{Handler: server.Handler()}
	server.Transition(runtime.WAITING)
	return server
}

/* listen activates the graph-declared endpoint exactly once, after configuration. */
func (server *HTTPServerServer) listen(address string) error {
	if address == "" {
		return nil
	}
	if server.listening {
		if address != server.httpServer.Addr {
			return errnie.Error(errnie.Err(errnie.Validation, "http.server: listening address cannot change", nil))
		}
		return nil
	}
	if server.bind != nil {
		return server.bind
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		server.bind = errnie.Error(errnie.Err(errnie.IO, "http.server: failed to listen on "+address, err))
		return server.bind
	}
	server.httpServer.Addr = address
	server.address = listener.Addr().String()
	server.listening = true
	go func() {
		err := server.httpServer.Serve(listener)
		if err != nil && err != stdhttp.ErrServerClosed {
			errnie.Error(errnie.Err(errnie.IO, "http.server: serve", err))
		}
	}()
	go func() {
		<-server.Context().Done()
		if err := server.httpServer.Close(); err != nil {
			errnie.Error(errnie.Err(errnie.IO, "http.server: stop listener", err))
		}
	}()
	server.Transition(runtime.READY)
	return nil
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

	for _, route := range []string{
		"POST /workbench/query", "GET /hindsight/metric-map", "GET /hindsight/runs",
		"GET /hindsight/symbols", "GET /hindsight/excursions", "GET /trades",
	} {
		mux.Handle(route, &server.inspection)
	}

	// Mount WebRTC and WebSocket handlers
	mux.HandleFunc("POST /fluid/webrtc/offer", server.webrtcServer.OfferHandler())
	mux.HandleFunc("GET /ws", server.WebSocketServerServer.UpgradeHandler())
	mux.HandleFunc("GET /hindsight/timeline", server.WebSocketServerServer.UpgradeHandler())

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
	routes, err := call.Args().Routes()
	if err != nil {
		return errnie.Error(err)
	}
	if err := server.inspection.configure(call.Args().Query(), routes); err != nil {
		return err
	}
	address, err := call.Args().Address()
	if err != nil {
		return errnie.Error(err)
	}
	if err := server.listen(address); err != nil {
		return err
	}
	data, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"http.server: failed to read data arg",
			err,
		))
	}

	if len(data) > 0 {
		server.WebSocketServerServer.Broadcast(data)
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
	if server.bind != nil {
		return server.Error(server.bind)
	}

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
WebRTCServer returns the underlying WebRTC server component.
*/
func (server *HTTPServerServer) WebRTCServer() *webrtc.WebRTCServerServer {
	return server.webrtcServer
}

/*
Close stops the HTTP server and associated protocol sub-servers.
*/
func (server *HTTPServerServer) Close() error {
	server.inspection.Close()
	var err error
	if server.httpServer != nil {
		err = server.httpServer.Close()
	}
	return errnie.Error(errors.Join(err, server.WebSocketServerServer.Close(), server.webrtcServer.Close(), server.System.Close()))
}

/* Shutdown releases the server with the lifetime of its Cap'n Proto capability. */
func (server *HTTPServerServer) Shutdown() {
	if err := server.Close(); err != nil {
		errnie.Error(err)
	}
}
