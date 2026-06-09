package ui

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/fasthttp/websocket"
	disruptor "github.com/smarty/go-disruptor"
	"github.com/theapemachine/errnie"
)

const frontendOutboundBufferSize = 4096

type outboundEvent struct {
	value any
}

/*
frontendLink is the single dashboard websocket and its outbound disruptor.
*/
type frontendLink struct {
	hub        *Hub
	conn       *websocket.Conn
	outbound   disruptor.Disruptor
	ring       []outboundEvent
	handler    *frontendOutboundHandler
	skipOldest atomic.Int64
	waitGroup  sync.WaitGroup
}

type frontendOutboundHandler struct {
	link      *frontendLink
	processed atomic.Int64
}

func (handler *frontendOutboundHandler) Handle(lowerSequence, upperSequence int64) {
	for sequence := lowerSequence; sequence <= upperSequence; sequence++ {
		if handler.link.consumeSkip() {
			handler.processed.Store(sequence)

			continue
		}

		event := handler.link.ring[sequence&(frontendOutboundBufferSize-1)]

		if writeErr := handler.link.conn.WriteJSON(event.value); writeErr != nil {
			handler.link.hub.detachFrontend(handler.link)
			errnie.Error(writeErr)

			return
		}

		handler.processed.Store(sequence)
	}
}

func (hub *Hub) newFrontendLink(conn *websocket.Conn) (*frontendLink, error) {
	link := &frontendLink{
		hub:  hub,
		conn: conn,
		ring: make([]outboundEvent, frontendOutboundBufferSize),
	}
	handler := &frontendOutboundHandler{link: link}

	instance, err := disruptor.New(
		disruptor.Options.BufferCapacity(frontendOutboundBufferSize),
		disruptor.Options.WriterCount(1),
		disruptor.Options.NewHandlerGroup(handler),
	)

	if err != nil {
		return nil, fmt.Errorf("ui: initialize frontend outbound disruptor: %w", err)
	}

	link.outbound = instance
	link.handler = handler
	link.waitGroup.Add(1)

	go func() {
		defer link.waitGroup.Done()
		instance.Listen()
	}()

	return link, nil
}

func (hub *Hub) detachFrontend(link *frontendLink) {
	if link == nil {
		return
	}

	if hub.frontend.CompareAndSwap(link, nil) {
		link.close()

		return
	}

	link.close()
}

func (link *frontendLink) close() {
	if link.outbound != nil {
		_ = link.outbound.Close()
		link.waitGroup.Wait()
	}

	if link.conn != nil {
		errnie.Error(link.conn.Close())
	}
}

func (link *frontendLink) consumeSkip() bool {
	for {
		remaining := link.skipOldest.Load()

		if remaining <= 0 {
			return false
		}

		if link.skipOldest.CompareAndSwap(remaining, remaining-1) {
			return true
		}
	}
}

/*
publish reserves a ring slot via TryReserve, writes the frame, and commits.
When the ring is full the producer signals the frontend writer to skip the oldest
pending frame and retries without blocking the ui bus subscriber.
*/
func (link *frontendLink) publish(value any) error {
	event := outboundEvent{value: value}

	for attempt := 0; attempt < frontendOutboundBufferSize; attempt++ {
		upperSequence := link.outbound.TryReserve(1)

		switch upperSequence {
		case disruptor.ErrCapacityUnavailable:
			link.skipOldest.Add(1)
			continue
		case disruptor.ErrReservationSize:
			return fmt.Errorf("ui: invalid frontend outbound reservation")
		default:
			link.ring[upperSequence&(frontendOutboundBufferSize-1)] = event
			link.outbound.Commit(upperSequence, upperSequence)

			return nil
		}
	}

	return nil
}
