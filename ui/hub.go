package ui

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/theapemachine/errnie"
)

const clientSendBuffer = 256
const priceTickFlushEvery = 50 * time.Millisecond

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: allowLocalhostOrigin,
}

func allowLocalhostOrigin(request *http.Request) bool {
	origin := strings.TrimSpace(request.Header.Get("Origin"))

	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)

	if err != nil {
		return false
	}

	host := strings.ToLower(parsed.Hostname())

	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

type wsClient struct {
	conn         *websocket.Conn
	send         chan []byte
	mu           sync.Mutex
	symbols      map[string]struct{}
	tickMu       sync.Mutex
	pendingTicks map[string][]byte
}

func (client *wsClient) subscribe(symbols []string) {
	client.mu.Lock()
	defer client.mu.Unlock()

	if client.symbols == nil {
		client.symbols = make(map[string]struct{})
	}

	for _, symbol := range symbols {
		symbol = strings.TrimSpace(symbol)
		if symbol == "" {
			continue
		}

		client.symbols[symbol] = struct{}{}
	}
}

func (client *wsClient) unsubscribe(symbols []string) {
	client.mu.Lock()
	defer client.mu.Unlock()

	for _, symbol := range symbols {
		symbol = strings.TrimSpace(symbol)
		if symbol == "" {
			continue
		}

		delete(client.symbols, symbol)
	}
}

func (client *wsClient) wantsSymbol(symbol string) bool {
	client.mu.Lock()
	defer client.mu.Unlock()

	if len(client.symbols) == 0 {
		return false
	}

	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return false
	}

	_, ok := client.symbols[symbol]

	return ok
}

/*
Hub sends telemetry to WebSocket clients (SciCharts / monitor UI).
*/
type Hub struct {
	ctx          context.Context
	cancel       context.CancelFunc
	clients      sync.Map
	bootstrap    func() []map[string]any
	replayMu     sync.Mutex
	replayByType map[string]map[string]any
	runID        string
}

var replayEventTypes = []string{
	"field_snapshot",
	"engine_pulse",
	"decision_trace",
	"scoreboard",
	"status",
}

/*
NewHub creates a new telemetry hub.
*/
func NewHub(ctx context.Context, bootstrap func() []map[string]any) (*Hub, error) {
	ctx, cancel := context.WithCancel(ctx)

	hub := &Hub{
		ctx:          ctx,
		cancel:       cancel,
		bootstrap:    bootstrap,
		replayByType: make(map[string]map[string]any),
		runID:        time.Now().UTC().Format("20060102T150405Z"),
	}

	go hub.runPriceTickFlush()

	return hub, errnie.Require(map[string]any{
		"ctx":    hub.ctx,
		"cancel": hub.cancel,
	})
}

/*
Serve starts the websocket server on addr (e.g. :8765).
*/
func (hub *Hub) Serve(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.handleWS)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		<-hub.ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	return server.ListenAndServe()
}

/*
Emit publishes a flat JSON telemetry event to all connected clients without blocking.
*/
func (hub *Hub) Emit(event map[string]any) {
	hub.storeReplay(event)

	payload, err := json.Marshal(event)
	if err != nil {
		_ = errnie.Error(err)
		return
	}

	eventName, _ := event["event"].(string)
	tickSymbol, _ := event["symbol"].(string)

	if eventName == "price_tick" {
		hub.coalescePriceTick(tickSymbol, payload)
		return
	}

	hub.clients.Range(func(key, value any) bool {
		client := key.(*wsClient)

		hub.deliver(client, payload, eventIsCritical(eventName))

		return true
	})
}

func (hub *Hub) runPriceTickFlush() {
	ticker := time.NewTicker(priceTickFlushEvery)
	defer ticker.Stop()

	for {
		select {
		case <-hub.ctx.Done():
			return
		case <-ticker.C:
			hub.flushPendingTicks()
		}
	}
}

func (hub *Hub) coalescePriceTick(symbol string, payload []byte) {
	if symbol == "" {
		return
	}

	hub.clients.Range(func(key, value any) bool {
		client := key.(*wsClient)

		if !client.wantsSymbol(symbol) {
			return true
		}

		client.tickMu.Lock()

		if client.pendingTicks == nil {
			client.pendingTicks = make(map[string][]byte)
		}

		client.pendingTicks[symbol] = payload
		client.tickMu.Unlock()

		return true
	})
}

func (hub *Hub) flushPendingTicks() {
	hub.clients.Range(func(key, value any) bool {
		client := key.(*wsClient)

		client.tickMu.Lock()
		pending := client.pendingTicks
		client.pendingTicks = nil
		client.tickMu.Unlock()

		for _, payload := range pending {
			hub.deliver(client, payload, false)
		}

		return true
	})
}

func eventIsCritical(eventName string) bool {
	switch eventName {
	case "hello", "status", "trade_enter", "trade_exit", "stop_ratchet":
		return true
	default:
		return false
	}
}

func (hub *Hub) deliver(client *wsClient, payload []byte, critical bool) {
	if critical {
		client.send <- payload
		return
	}

	select {
	case client.send <- payload:
	default:
	}
}

func (hub *Hub) handleWS(writer http.ResponseWriter, request *http.Request) {
	conn, err := wsUpgrader.Upgrade(writer, request, nil)

	if err != nil {
		_ = errnie.Error(err)
		return
	}

	client := &wsClient{
		conn: conn,
		send: make(chan []byte, clientSendBuffer),
	}

	hub.clients.Store(client, struct{}{})
	hub.sendHello(client)
	hub.sendBootstrap(client)

	go hub.writePump(client)
	hub.readPump(client)
}

func (hub *Hub) sendHello(client *wsClient) {
	hub.enqueue(client, map[string]any{
		"event":  "hello",
		"ts":     time.Now().UTC().Format(time.RFC3339Nano),
		"run_id": hub.runID,
	})
}

func (hub *Hub) sendBootstrap(client *wsClient) {
	if hub.bootstrap != nil {
		for _, event := range hub.bootstrap() {
			if event == nil {
				continue
			}

			hub.enqueue(client, event)
		}
	}

	for _, event := range hub.replayEvents() {
		hub.enqueue(client, event)
	}
}

func (hub *Hub) storeReplay(event map[string]any) {
	eventName, ok := event["event"].(string)
	if !ok {
		return
	}

	for _, replayType := range replayEventTypes {
		if eventName != replayType {
			continue
		}

		hub.replayMu.Lock()
		hub.replayByType[eventName] = event
		hub.replayMu.Unlock()

		return
	}
}

func (hub *Hub) replayEvents() []map[string]any {
	hub.replayMu.Lock()
	defer hub.replayMu.Unlock()

	events := make([]map[string]any, 0, len(replayEventTypes))

	for _, replayType := range replayEventTypes {
		event, ok := hub.replayByType[replayType]
		if !ok || event == nil {
			continue
		}

		events = append(events, event)
	}

	return events
}

func (hub *Hub) enqueue(client *wsClient, event map[string]any) {
	payload, err := json.Marshal(event)
	if err != nil {
		_ = errnie.Error(err)
		return
	}

	eventName, _ := event["event"].(string)
	hub.deliver(client, payload, eventIsCritical(eventName))
}

func (hub *Hub) writePump(client *wsClient) {
	defer func() {
		hub.clients.Delete(client)
		_ = client.conn.Close()
	}()

	for {
		select {
		case <-hub.ctx.Done():
			return
		case payload, ok := <-client.send:
			if !ok {
				return
			}

			if err := client.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		}
	}
}

func (hub *Hub) readPump(client *wsClient) {
	defer func() {
		hub.clients.Delete(client)
		close(client.send)
	}()

	for {
		if hub.ctx.Err() != nil {
			return
		}

		_, payload, err := client.conn.ReadMessage()
		if err != nil {
			return
		}

		hub.handleClientMessage(client, payload)
	}
}

type clientMessage struct {
	Op      string   `json:"op"`
	Symbols []string `json:"symbols"`
	Symbol  string   `json:"symbol"`
}

func (hub *Hub) handleClientMessage(client *wsClient, payload []byte) {
	var message clientMessage

	if err := json.Unmarshal(payload, &message); err != nil {
		return
	}

	switch message.Op {
	case "subscribe":
		client.subscribe(message.Symbols)
		if strings.TrimSpace(message.Symbol) != "" {
			client.subscribe([]string{message.Symbol})
		}
	case "unsubscribe":
		client.unsubscribe(message.Symbols)
		if strings.TrimSpace(message.Symbol) != "" {
			client.unsubscribe([]string{message.Symbol})
		}
	case "watch":
		if strings.TrimSpace(message.Symbol) != "" {
			client.subscribe([]string{message.Symbol})
		}
	}
}

/*
SetBootstrap wires the connect-time snapshot for new websocket clients.
*/
func (hub *Hub) SetBootstrap(bootstrap func() []map[string]any) {
	hub.bootstrap = bootstrap
}

/*
Close shuts down the telemetry hub.
*/
func (hub *Hub) Close() error {
	hub.cancel()

	return errnie.Require(map[string]any{
		"event": "ui_hub_closed",
	})
}

/*
ListenAddr parses config.System.UIAddr into a host listen address (e.g. :8765).
*/
func ListenAddr(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	if strings.HasPrefix(raw, ":") {
		return net.JoinHostPort("127.0.0.1", strings.TrimPrefix(raw, ":")), true
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false
	}

	if parsed.Host != "" {
		host := parsed.Host
		if strings.Contains(host, ":") {
			return host, true
		}

		return net.JoinHostPort(host, "8765"), true
	}

	return "", false
}
