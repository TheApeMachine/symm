package trader

import (
	"context"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
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
	ui          *transport.MapReduce[[]byte]
	manifold    *transport.MapReduce[types.FluidFrame]
	thesis      *types.Thesis
	recorder    *audit.Recorder
	desk        *broker.Desk
	diagnostics *Diagnostics
}

/*
NewCrypto constructs Crypto; Boot Initialize attaches planner and desk.
*/
func NewCrypto(
	ctx context.Context,
	api *websocket.API,
	ui *transport.MapReduce[[]byte],
	manifold *transport.MapReduce[types.FluidFrame],
	recorder *audit.Recorder,
	desk *broker.Desk,
	thesis *types.Thesis,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:      ctx,
		cancel:   cancel,
		api:      api,
		ui:       ui,
		manifold: manifold,
		thesis:   thesis,
		recorder: recorder,
		desk:     desk,
		diagnostics: &Diagnostics{
			started: time.Now(),
		},
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

func (crypto *Crypto) Run() error {
	for crypto.err == nil {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		default:
			crypto.thesis.Tick++

			crypto.thesis.Symbols.Range(func(key, value any) bool {
				symbol, ok := value.(*types.Symbol)

				if !ok || symbol == nil || symbol.Decisions.Length() == 0 {
					runtime.Gosched()
					return true
				}

				// Process each symbol here.
				return true
			})

			utils.Publish(
				crypto.ui, datura.NewMap(
					"tick", datura.NewMap("count", crypto.thesis.Tick),
				),
			)
		}
	}

	return crypto.err
}

func (crypto *Crypto) Close() error {
	crypto.cancel()
	return nil
}
