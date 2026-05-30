package trader

import (
	"context"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/order"
)

/*
paperSession simulates the live submit → ack → fill pipeline on paper wallets.
*/
type paperSession struct {
	ctx    context.Context
	cancel context.CancelFunc
	orderSession
	fills chan order.Fill
	acks  chan order.Ack
}

/*
NewPaperSession starts the paper execution simulator.
*/
func NewPaperSession(ctx context.Context) *paperSession {
	ctx, cancel := context.WithCancel(ctx)

	return &paperSession{
		ctx:    ctx,
		cancel: cancel,
		fills:  make(chan order.Fill, 128),
		acks:   make(chan order.Ack, 64),
	}
}

/*
Fills exposes simulated trade executions.
*/
func (session *paperSession) Fills() <-chan order.Fill {
	return session.fills
}

/*
Acks exposes simulated order rejects (live parity).
*/
func (session *paperSession) Acks() <-chan order.Ack {
	return session.acks
}

/*
Close stops delayed fill scheduling.
*/
func (session *paperSession) Close() error {
	session.cancel()

	return session.ctx.Err()
}

/*
EnqueueReject schedules a reject ack matching the live desk path.
*/
func (session *paperSession) EnqueueReject(clOrdID string, reason string) {
	session.enqueueAck(order.Ack{
		Method:  order.MethodAddOrder,
		Success: false,
		Error:   reason,
		Result: struct {
			OrderID      string `json:"order_id"`
			ClOrdID      string `json:"cl_ord_id"`
			OrderUserref int    `json:"order_userref"`
		}{ClOrdID: clOrdID},
	})
}

/*
ScheduleFill delivers one fill after PaperOrderLatency (immediate when zero).
*/
func (session *paperSession) ScheduleFill(fill order.Fill) {
	delay := config.System.PaperOrderLatency

	if delay <= 0 {
		session.enqueueFill(fill)

		return
	}

	time.AfterFunc(delay, func() {
		select {
		case <-session.ctx.Done():
			return
		default:
			session.enqueueFill(fill)
		}
	})
}

func (session *paperSession) enqueueFill(fill order.Fill) {
	select {
	case session.fills <- fill:
	case <-session.ctx.Done():
	}
}

func (session *paperSession) enqueueAck(ack order.Ack) {
	select {
	case session.acks <- ack:
	case <-session.ctx.Done():
	}
}
