package leadlag

import (
	"context"
	"sync/atomic"

	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the Anchor perspective, measuring temporal correlation between the
cross-section leader and each follower. Categories belong in logic; this signal
emits numerical scores only.
*/
type Signal struct {
	status  atomic.Value
	ctx     context.Context
	cancel  context.CancelFunc
	api     *websocket.API
	section *Section
	ui      chan []byte
}

/*
NewSignal creates lead-lag measurement state for central market cuts so
temporal relationships persist across Thesis ticks.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:     ctx,
		cancel:  cancel,
		api:     api,
		section: NewSection(),
		ui:      ui,
	}

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceLeadLag)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourceLeadLag
}

func (signal *Signal) Status() types.Status {
	return signal.status.Load().(types.Status)
}

func (signal *Signal) Measure(thesis *types.Thesis) ([]*types.Measurement, bool) {
	if signal.section == nil {
		signal.section = NewSection()
	}

	tickers := thesis.MarketTickers(types.SourceLeadLag)

	return signal.measureFrame(tickers), true
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
