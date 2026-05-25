package trader

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
)

/*
Crypto executes portfolio decisions from scored candidate frames.
*/
type Crypto struct {
	ctx          context.Context
	cancel       context.CancelFunc
	pool         *qpool.Q
	broadcasts   map[string]*qpool.BroadcastGroup
	subscribers  map[string]*qpool.Subscriber
	wallet       *Wallet
	portfolio    *Portfolio
	measurements []engine.Measurement
}

/*
NewCrypto creates a trader that decides from broadcast candidate frames.
*/
func NewCrypto(
	ctx context.Context,
	pool *qpool.Q,
	wallet *Wallet,
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		broadcasts:   make(map[string]*qpool.BroadcastGroup),
		subscribers:  make(map[string]*qpool.Subscriber),
		wallet:       wallet,
		portfolio:    NewPortfolio(wallet),
		measurements: make([]engine.Measurement, 0),
	}

	crypto.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	crypto.subscribers["measurements"] = crypto.broadcasts["measurements"].Subscribe("measurements", 128)

	if errnie.Error(errnie.Require(map[string]any{
		"ctx":       ctx,
		"cancel":    cancel,
		"pool":      pool,
		"wallet":    wallet,
		"portfolio": crypto.portfolio,
	})) != nil {
		return nil
	}

	return crypto
}

/*
Start arms the decision ticker and candidate subscription.
*/
func (crypto *Crypto) Start() error {
	return nil
}

/*
State reports whether the trader is ready for decision ticks.
*/
func (crypto *Crypto) State() engine.State {
	return engine.READY
}

/*
Tick buffers candidate frames and runs decisions on the interval.
*/
func (crypto *Crypto) Tick() error {
	select {
	case <-crypto.ctx.Done():
		crypto.cancel()
		return crypto.ctx.Err()
	case measurement := <-crypto.subscribers["measurements"].Incoming:
		payload, ok := measurement.Value.(engine.Measurement)

		if !ok {
			return errnie.Error(fmt.Errorf("invalid measurement: %v", measurement.Value))
		}

		crypto.measurements = append(crypto.measurements, payload)

		return nil
	default:
		for _, measurement := range crypto.measurements {
			switch measurement.Type {
			case engine.Pump:
				// TODO: Implement pump logic
			case engine.Dump:
				// TODO: Implement dump logic
			case engine.Momentum:
				// TODO: Implement momentum logic
			case engine.Flow:
				// TODO: Implement flow logic
			case engine.Causal:
				// TODO: Implement causal logic
			case engine.DepthFlow:
				// TODO: Implement depth flow logic
			case engine.LeadLag:
				// TODO: Implement lead lag logic
			case engine.Basis:
				// TODO: Implement basis logic
			case engine.Sentiment:
				// TODO: Implement sentiment logic
			}
		}

		// Clear measurements for next tick, if we have made a decision.
		crypto.measurements = crypto.measurements[:0]

		return nil
	}
}

/*
Close stops the trader context and decision ticker.
*/
func (crypto *Crypto) Close() error {
	crypto.cancel()
	return nil
}
