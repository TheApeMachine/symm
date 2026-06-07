package public

import "sync"

var (
	reconnectHandlersMu sync.RWMutex
	reconnectHandlers   []func()
)

/*
OnReconnect registers a callback invoked after the public websocket dials successfully.
Handlers replay channel subscriptions so a reconnect restores market data immediately.
*/
func OnReconnect(handler func()) {
	if handler == nil {
		return
	}

	reconnectHandlersMu.Lock()
	reconnectHandlers = append(reconnectHandlers, handler)
	reconnectHandlersMu.Unlock()
}

func notifyReconnect() {
	reconnectHandlersMu.RLock()
	handlers := append([]func(){}, reconnectHandlers...)
	reconnectHandlersMu.RUnlock()

	for _, handler := range handlers {
		handler()
	}
}
