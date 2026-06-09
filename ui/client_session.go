package ui

import (
	"context"
	"sync"

	"github.com/fasthttp/websocket"
	"github.com/theapemachine/errnie"
)

const clientOutboundBufferSize = 4096

type outboundEvent struct {
	value any
}

type clientSession struct {
	hub      *Hub
	connID   uint64
	conn     *websocket.Conn
	outbound chan outboundEvent
	cancel   context.CancelFunc
	waitGroup sync.WaitGroup
}

func (hub *Hub) attachClient(connID uint64, conn *websocket.Conn) error {
	outboundCtx, cancel := context.WithCancel(hub.ctx)
	session := &clientSession{
		hub:      hub,
		connID:   connID,
		conn:     conn,
		outbound: make(chan outboundEvent, clientOutboundBufferSize),
		cancel:   cancel,
	}

	session.waitGroup.Add(1)

	go func() {
		defer session.waitGroup.Done()
		session.drainOutbound(outboundCtx)
	}()

	hub.sessions.Store(connID, session)

	return nil
}

func (session *clientSession) drainOutbound(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-session.outbound:
			if writeErr := session.conn.WriteJSON(event.value); writeErr != nil {
				session.hub.detachClient(session.connID)
				errnie.Error(writeErr)

				return
			}
		}
	}
}

func (hub *Hub) detachClient(connID uint64) {
	raw, ok := hub.sessions.LoadAndDelete(connID)

	if !ok {
		hub.clients.Delete(connID)
		return
	}

	session, ok := raw.(*clientSession)

	if !ok {
		hub.clients.Delete(connID)
		return
	}

	session.cancel()
	session.waitGroup.Wait()
	hub.clients.Delete(connID)
}

/*
publish enqueues a frame for the browser writer. When the ring is full the oldest
pending frame is dropped so the hub never blocks on slow clients.
*/
func (session *clientSession) publish(value any) error {
	event := outboundEvent{value: value}

	select {
	case session.outbound <- event:
		return nil
	default:
	}

	select {
	case <-session.outbound:
	default:
	}

	select {
	case session.outbound <- event:
		return nil
	default:
	}

	return nil
}
