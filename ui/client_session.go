package ui

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/fasthttp/websocket"
	disruptor "github.com/smarty/go-disruptor"
	"github.com/theapemachine/errnie"
)

const clientOutboundBufferSize = 4096

type clientSession struct {
	hub       *Hub
	connID    uint64
	conn      *websocket.Conn
	outbound  disruptor.Disruptor
	ring      []outboundEvent
	handler   *clientOutboundHandler
	waitGroup sync.WaitGroup
}

type clientOutboundHandler struct {
	session   *clientSession
	processed atomic.Int64
}

func (handler *clientOutboundHandler) Handle(lowerSequence, upperSequence int64) {
	for sequence := lowerSequence; sequence <= upperSequence; sequence++ {
		event := handler.session.ring[sequence&(clientOutboundBufferSize-1)]

		if writeErr := handler.session.conn.WriteJSON(event.value); writeErr != nil {
			handler.session.hub.detachClient(handler.session.connID)
			errnie.Error(writeErr)

			return
		}

		handler.processed.Store(sequence)
	}
}

func (hub *Hub) attachClient(connID uint64, conn *websocket.Conn) error {
	session := &clientSession{
		hub:    hub,
		connID: connID,
		conn:   conn,
		ring:   make([]outboundEvent, clientOutboundBufferSize),
	}
	handler := &clientOutboundHandler{session: session}

	instance, err := disruptor.New(
		disruptor.Options.BufferCapacity(clientOutboundBufferSize),
		disruptor.Options.WriterCount(1),
		disruptor.Options.NewHandlerGroup(handler),
	)

	if err != nil {
		return fmt.Errorf("ui: initialize client outbound disruptor: %w", err)
	}

	session.outbound = instance
	session.handler = handler
	session.waitGroup.Add(1)

	go func() {
		defer session.waitGroup.Done()
		instance.Listen()
	}()

	hub.sessions.Store(connID, session)

	return nil
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

	if session.outbound != nil {
		_ = session.outbound.Close()
		session.waitGroup.Wait()
	}

	hub.clients.Delete(connID)
}

func (session *clientSession) publish(value any) error {
	event := outboundEvent{value: value}

	for attempt := 0; attempt < 2; attempt++ {
		upperSequence := session.outbound.TryReserve(1)

		switch upperSequence {
		case disruptor.ErrCapacityUnavailable:
			session.dropOldestOutbound(event)
			continue
		case disruptor.ErrReservationSize:
			return fmt.Errorf("ui: invalid client outbound reservation")
		default:
			session.ring[upperSequence&(clientOutboundBufferSize-1)] = event
			session.outbound.Commit(upperSequence, upperSequence)

			return nil
		}
	}

	return fmt.Errorf("ui: client outbound saturated")
}

func (session *clientSession) dropOldestOutbound(event outboundEvent) {
	processed := session.handler.processed.Load()
	oldest := processed + 1

	session.ring[oldest&(clientOutboundBufferSize-1)] = event
	session.outbound.Commit(oldest, oldest)
}
