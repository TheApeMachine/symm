package controlflow

import (
	"bytes"
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
DelayServer paces incoming data payloads by enforcing a minimum millisecond
delay between consecutive emissions to downstream components.
*/
type DelayServer struct {
	*runtime.System
	millis   int64
	lastEmit time.Time
	data     []byte
	ready    bool
}

func NewDelay(ctx context.Context) *DelayServer {
	server := &DelayServer{
		System: runtime.NewSystem(ctx, "controlflow.delay"),
		millis: 100,
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write accepts an inbound data payload and optional millisecond pace limit.
*/
func (server *DelayServer) Write(ctx context.Context, call Delay_write) error {
	millis := call.Args().Millis()

	if millis > 0 {
		server.millis = millis
	}

	data, err := call.Args().Data()

	if err == nil && len(data) > 0 {
		server.data = bytes.Clone(data)
	}

	now := time.Now()
	interval := time.Duration(server.millis) * time.Millisecond
	canEmit := server.lastEmit.IsZero() || now.Sub(server.lastEmit) >= interval

	server.ready = canEmit

	if canEmit {
		server.lastEmit = now
	}

	return nil
}

/*
Done emits the paced data when the delay threshold has elapsed.
*/
func (server *DelayServer) Done(ctx context.Context, call Delay_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"controlflow.delay: failed to allocate done results",
			err,
		))
	}

	results.SetReady(server.ready)

	if server.ready && len(server.data) > 0 {
		server.ready = false

		if err := results.SetOut(server.data); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"controlflow.delay: failed to set out",
				err,
			))
		}
	}

	return nil
}
