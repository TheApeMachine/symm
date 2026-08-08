package leadlag

import (
	"context"

	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the Anchor perspective, measuring temporal correlation between the
cross-section leader and each follower. Categories belong in logic; this signal
emits numerical scores only.
*/
type Signal struct {
	status    types.Status
	ctx       context.Context
	cancel    context.CancelFunc
	api       *websocket.API
	section   *Section
	ui        chan []byte
	thesis    *types.Thesis
	semaphore chan struct{}
}

/*
NewSignal creates lead-lag measurement state for central market cuts so
temporal relationships persist across Thesis ticks.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
	thesis *types.Thesis,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		status:    types.INITIALIZING,
		ctx:       ctx,
		cancel:    cancel,
		api:       api,
		section:   NewSection(),
		ui:        ui,
		thesis:    thesis,
		semaphore: make(chan struct{}, 1),
	}

	signal.thesis.Subscribe(types.SourceLeadLag, signal.semaphore)
	signal.run()
	signal.status = types.READY

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceLeadLag)
}

func (signal *Signal) Status() types.Status {
	return signal.status
}

func (signal *Signal) run() {
	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case <-signal.semaphore:
				signal.thesis.AppendMeasurements(
					signal.Measure(signal.thesis), true,
				)
			}
		}
	}()
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	if signal.section == nil {
		signal.section = NewSection()
	}

	tickers := thesis.MarketTickers(types.SourceLeadLag)

	return signal.measureFrame(tickers)
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
