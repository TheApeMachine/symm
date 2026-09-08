package replay

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/theapemachine/errnie"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"golang.org/x/sync/errgroup"
)

// Observer reads the real dashboard output, including the completed learning
// stage's counter. This is downstream of every production workspace stage.
type Observer struct {
	Completed atomic.Int64
	Steps     atomic.Uint64
	Decisions atomic.Uint64
	Markets   atomic.Int64
	Agents    atomic.Int64
	First     atomic.Int64
	Last      atomic.Int64
	Backlog   atomic.Int64
}

func (observer *Observer) Read(ctx context.Context, url string, cadence time.Duration) error {
	group, ctx := errgroup.WithContext(ctx)
	ready := make(chan struct{})
	group.Go(func() error { return observer.readWebsocket(ctx, url, cadence, ready) })
	group.Go(func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ready:
		}
		return observer.readDiagnostics(ctx, "http"+strings.TrimSuffix(strings.TrimPrefix(url, "ws"), "/ws")+"/webrtc/manifold")
	})
	return group.Wait()
}

func (observer *Observer) readWebsocket(ctx context.Context, url string, cadence time.Duration, ready chan<- struct{}) error {
	var connection *websocket.Conn
	clock := time.NewTicker(cadence)
	defer clock.Stop()

	for {
		candidate, response, err := websocket.DefaultDialer.DialContext(ctx, url, nil)

		if response != nil {
			if err := response.Body.Close(); err != nil {
				return errnie.Error(err)
			}
		}

		if err == nil {
			connection = candidate
			close(ready)
			break
		}

		if !errors.Is(err, syscall.ECONNREFUSED) {
			return errnie.Error(err)
		}

		// The production hub listens only after the subscription readiness barrier.
		select {
		case <-ctx.Done():
			return errnie.Error(ctx.Err())
		case <-clock.C:
		}
	}
	stop := context.AfterFunc(ctx, func() {
		if err := connection.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			errnie.Error(err)
		}
	})
	defer stop()
	defer func() {
		if err := connection.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			errnie.Error(err)
		}
	}()

	for {
		_, payload, err := connection.ReadMessage()

		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errnie.Error(err)
		}
		state := wire.GetRootAsEnvelopeState(payload, 0)

		if reading := state.LearningBytes(); len(reading) > 0 {
			learning := wire.GetRootAsLearningState(reading, 0)
			observer.Steps.Store(learning.Steps())
			observer.Decisions.Store(learning.Decisions())
			observer.Markets.Store(int64(learning.MarketsLength()))
			observer.Agents.Store(int64(learning.AgentsLength()))
		}
	}
}
