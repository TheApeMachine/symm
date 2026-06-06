package private

import (
	"context"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
)

/*
hybridWebSocket runs paper execution and a live data-only L3 feed together.
*/
type hybridWebSocket struct {
	ctx   context.Context
	paper public.WebSocketClient
	l3    *WebSocket
}

func newHybridWebSocket(
	ctx context.Context,
	pool *qpool.Q,
	apiKey string,
	apiSecret string,
	paperClient public.WebSocketClient,
) public.WebSocketClient {
	l3Client, err := newDataOnlyWebSocket(ctx, pool, apiKey, apiSecret)

	if err != nil {
		errnie.Error(err, "kraken/private: L3 data socket unavailable, paper only")

		return paperClient
	}

	errnie.Info("kraken/private hybrid paper execution + live L3 data", "kraken/private")

	return &hybridWebSocket{
		ctx:   ctx,
		paper: paperClient,
		l3:    l3Client,
	}
}

func (hybrid *hybridWebSocket) Connect(
	endpoint public.EndpointType, channel string, n uint64,
) error {
	if hybrid.l3 != nil {
		if err := hybrid.l3.Connect(public.WebSocketL3URL, public.Level3Channel, n); err != nil {
			return err
		}
	}

	return hybrid.paper.Connect(endpoint, channel, n)
}

func (hybrid *hybridWebSocket) Tick() error {
	if hybrid.l3 == nil {
		return hybrid.paper.Tick()
	}

	errCh := make(chan error, 2)
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)

	go func() {
		defer waitGroup.Done()
		errCh <- hybrid.paper.Tick()
	}()

	go func() {
		defer waitGroup.Done()
		errCh <- hybrid.l3.Tick()
	}()

	select {
	case <-hybrid.ctx.Done():
		waitGroup.Wait()
		_ = hybrid.Close()
		return hybrid.ctx.Err()
	case err := <-errCh:
		waitGroup.Wait()
		_ = hybrid.Close()
		return err
	}
}

func (hybrid *hybridWebSocket) Close() error {
	var joined error

	if err := hybrid.paper.Close(); err != nil {
		joined = err
	}

	if hybrid.l3 != nil {
		if err := hybrid.l3.Close(); err != nil && joined == nil {
			joined = err
		}
	}

	return joined
}
