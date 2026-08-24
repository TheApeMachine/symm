package trader

import (
	"context"
	"github.com/theapemachine/symm/nomagique/runtime"
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
syncRest parks the replay sync poll between passes. Gosched spun the scheduler
through every poll (measured as the dominant profile entry during replay); the
queue depths change at tick pace, so a short rest is indistinguishable from
spinning to the pump and far cheaper to every other goroutine.
*/
const syncRest = time.Millisecond

/*
Crypto submits desk work from thesis messages delivered by the Actor cascade.
*/
type Crypto struct {
	status      atomic.Value
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	api         *websocket.API
	ui          *runtime.Channel[*types.UIFrame]
	fluid       *runtime.Channel[types.FluidFrame]
	bus         *runtime.Workspace
	thesis      *types.Thesis
	recorder    *audit.Recorder
	desk          *broker.Desk
	diagnostics   *Diagnostics
	diagnosticsCh *runtime.Channel[StreamDiagnostics]
}

/*
NewCrypto constructs Crypto; Boot Initialize attaches planner and desk.
*/
func NewCrypto(
	ctx context.Context,
	api *websocket.API,
	recorder *audit.Recorder,
	desk *broker.Desk,
	thesis *types.Thesis,
	bus *runtime.Workspace,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:      ctx,
		cancel:   cancel,
		api:      api,
		bus:      bus,
		thesis:   thesis,
		recorder: recorder,
		desk:     desk,
		diagnostics: &Diagnostics{
			started: time.Now(),
		},
	}

	if bus != nil {
		crypto.ui = runtime.ChannelOf[*types.UIFrame](
			bus, types.ChannelUI,
			func(frame *types.UIFrame) string { return "" },
		)
		crypto.fluid = runtime.ChannelOf[types.FluidFrame](
			bus, types.ChannelFluid,
			func(frame types.FluidFrame) string { return frame.Channel },
		)
		crypto.diagnosticsCh = runtime.ChannelOf[StreamDiagnostics](
			bus, types.ChannelDiagnostics,
			func(diag StreamDiagnostics) string { return "" },
		)
	}

	crypto.status.Store(types.READY)
	crypto.bindDiagnostics()

	return crypto, nil
}

func (crypto *Crypto) Name() string { return "crypto" }

func (crypto *Crypto) Error() error { return crypto.err }

func (crypto *Crypto) Status() types.Status {
	return crypto.status.Load().(types.Status)
}

/*
SetDiagnosticsEnabled switches the diagnostics collector on or off at runtime.
Passing false stops per-observation timing and drops the heartbeat to an idle
cadence, leaving near-zero overhead on the market data path.
*/
func (crypto *Crypto) SetDiagnosticsEnabled(enabled bool) {
	if crypto == nil || crypto.diagnostics == nil {
		return
	}

	if enabled {
		crypto.diagnostics.Enable()
		return
	}

	crypto.diagnostics.Disable()
}

/*
DiagnosticsEnabled reports whether the diagnostics collector is switched on.
*/
func (crypto *Crypto) DiagnosticsEnabled() bool {
	return crypto != nil && crypto.diagnostics != nil && crypto.diagnostics.Enabled()
}

/*
ObserveModule returns the diagnostics module clock hook so stages that live
outside the analyzer/measurements wiring (like the resonance solver) can still
report their per-step duration into the same clock bank.
*/
func (crypto *Crypto) ObserveModule() func(string, time.Duration) {
	if crypto == nil || crypto.diagnostics == nil {
		return nil
	}

	return crypto.diagnostics.applyModule
}

/*
ObserveHop returns the diagnostics handoff clock used by strategy stages whose
transitions happen synchronously rather than through a thesis work queue.
*/
func (crypto *Crypto) ObserveHop() func(string, string, time.Duration) {
	if crypto == nil || crypto.diagnostics == nil {
		return nil
	}

	return crypto.diagnostics.applyHop
}

func (crypto *Crypto) Run() error {
	<-crypto.ctx.Done()
	return crypto.ctx.Err()
}

func (crypto *Crypto) Close() error {
	crypto.cancel()
	return nil
}
