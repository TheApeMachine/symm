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
	ingress *lf.Queue[data.Publication]
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
		ingress:   lf.NewQueue[data.Publication](),
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
func (tee *UITee) Push(pub data.Publication) {
	if tee.Status() != nmruntime.READY {
		errnie.Warn("pushing to a non-ready system may have unintended consequences")
		return
	}

	// Spin-cycle, you're washed.
	for tee.switching.Load() {
		runtime.Gosched()
	}

	if pub.Measurement == nil {
		return
	}

	for _, filter := range tee.filters {
		if !filter(pub.Measurement) {
			return
		}
	}

	pub.Retain()
	tee.ingress.Enqueue(pub)
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

func (tee *UITee) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			for {
				pub, ok := tee.ingress.Dequeue()

				if !ok {
					break
				}

				pub.Release()
			}

			return
		default:
			tee.switcheroo()

			if tee.ingress.Length() == 0 {
				time.Sleep(10 * time.Millisecond)
				continue
			}
		}

		for tee.ingress.Length() > 0 {
			pub, ok := tee.ingress.Dequeue()

			if !ok {
				break
			}

			dropped := false

			for _, filter := range tee.filters {
				if !filter(pub.Measurement) {
					// When the pimp's in the crib ma...
					dropped = true
					pub.Release()
				}
			}

			if dropped {
				// ...Drop it like it's hot.
				continue
			}

			payload, err := types.EncodeMeasurements(
				[]*data.Measurement{pub.Measurement},
			)

			pub.Release()

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

		// We have unfinished business...
		drain := tee.ingress

		// They're FRESH! Exciting, they're so exciting to me!
		tee.ingress = lf.NewQueue[data.Publication]()
		tee.egress = lf.NewQueue[[]byte]()

		go func() {
			for drain.Length() > 0 {
				// You're all 86.
				pub, ok := drain.Dequeue()

				if !ok {
					continue
				}

				// Let my people go.
				pub.Release()
			}
		}()

		tee.switching.Store(false)
		errnie.Info("[tee] I does that shit.")
	}
}
