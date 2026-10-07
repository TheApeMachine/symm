package ui

import (
	"context"
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
	"golang.design/x/lockfree/lf"
)

/*
UITee is a concrete off-ramp that accepts *data.Measurement
and yields encoded FlatBuffer []byte frames for the dashboard websocket,
including ManifoldFrame payloads when a measurement carries a ManifoldState.
It satisfies runtime.Tee[*data.Measurement, []byte].
Encoding is executed in a dedicated background worker to eliminate serialization
jitter from the consumer fast path and immediately release retained publications.
*/
type UITee struct {
	*nmruntime.System
	ingress *lf.Queue[*data.Measurement]
	egress  *lf.Queue[[]byte]
	filters []func(
		measurement *data.Measurement,
	) bool
	route     string
	symbol    string
	switching atomic.Bool
}

/*
NewUITee creates a new wait-free UITee off-ramp
with background FlatBuffer encoding.
*/
func NewUITee(
	ctx context.Context,
	label string,
	filters ...func(
		measurement *data.Measurement,
	) bool,
) *UITee {
	if len(filters) == 0 {
		filters = []func(*data.Measurement) bool{types.Filters}
	}

	tee := &UITee{
		ingress:   lf.NewQueue[*data.Measurement](),
		egress:    lf.NewQueue[[]byte](),
		filters:   filters,
		route:     types.Route(),
		symbol:    types.Focus(),
		switching: atomic.Bool{},
	}

	tee.System = nmruntime.NewSystem(ctx, label, tee)
	go tee.worker(tee.Context())
	return tee
}

/*
Push receives measurements from the workspace. Raw venue feeds stay off the
dashboard websocket; the page route still selects which analytical sources
are on the wire.
*/
func (tee *UITee) Push(measurement *data.Measurement) {
	if tee.Status() != nmruntime.READY {
		errnie.Warn("pushing to a non-ready system may have unintended consequences")
		return
	}

	// Spin-cycle, you're washed.
	for tee.switching.Load() {
		runtime.Gosched()
	}

	if measurement == nil {
		return
	}

	for _, filter := range tee.filters {
		if !filter(measurement) {
			return
		}
	}

	tee.ingress.Enqueue(measurement)
}

/*
Next yields the next available pre-encoded FlatBuffer frame ([]byte).
Encoding is owned exclusively by the background worker, so Next is an
instantaneous wait-free dequeue that preserves the worker's encode order.
*/
func (tee *UITee) Next() unsafe.Pointer {
	if tee.Status() != nmruntime.READY {
		errnie.Warn("[tee] pulling from a non-ready system may have unintended consequences")
		return nil
	}

	// Spin-cycle, you're washed.
	for tee.switching.Load() {
		runtime.Gosched()
	}

	payload, ok := tee.egress.Dequeue()

	if !ok || len(payload) == 0 {
		return nil
	}

	return unsafe.Pointer(&payload)
}

/*
IngressLength reports how many measurements are waiting in the ingress queue
to be encoded by the background worker.
*/
func (tee *UITee) IngressLength() int {
	if tee == nil || tee.ingress == nil {
		return 0
	}

	return int(tee.ingress.Length())
}

/*
EgressLength reports how many encoded FlatBuffer frames are queued waiting for
client transmission.
*/
func (tee *UITee) EgressLength() int {
	if tee == nil || tee.egress == nil {
		return 0
	}

	return int(tee.egress.Length())
}

/*
Pending returns the total backlog of un-transmitted items across ingress and egress.
*/
func (tee *UITee) Pending() int {
	return tee.IngressLength() + tee.EgressLength()
}

func (tee *UITee) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			tee.switcheroo()

			if tee.ingress.Length() == 0 {
				time.Sleep(10 * time.Millisecond)
				continue
			}
		}

		for tee.ingress.Length() > 0 {
			measurement, ok := tee.ingress.Dequeue()

			if !ok {
				break
			}

			dropped := false

			for _, filter := range tee.filters {
				if !filter(measurement) {
					// When the pimp's in the crib ma...
					dropped = true
				}
			}

			if dropped {
				// ...Drop it like it's hot.
				continue
			}

			payload, err := types.EncodeMeasurements(
				[]*data.Measurement{measurement},
			)

			if err != nil {
				tee.Error(errnie.Err(
					errnie.UnprocessableContent,
					"[tee] Failed to encode measurement",
					err,
				))
				continue
			}

			tee.egress.Enqueue(payload)
		}
	}
}

func (tee *UITee) switcheroo() {
	if types.Route() != tee.route || types.Focus() != tee.symbol {
		errnie.Info("[tee] I do... The Switcheroo!")
		tee.switching.Store(true)

		// Wait a minute...
		time.Sleep(10 * time.Millisecond)

		// K, go!
		tee.route = types.Route()
		tee.symbol = types.Focus()

		// They're FRESH! Exciting, they're so exciting to me!
		tee.ingress = lf.NewQueue[*data.Measurement]()
		tee.egress = lf.NewQueue[[]byte]()

		tee.switching.Store(false)
		errnie.Info("[tee] I does that shit.")
	}
}
