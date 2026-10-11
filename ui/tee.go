package ui

import (
	"context"
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
	ingress atomic.Pointer[lf.Queue[*data.Measurement]]
	egress  atomic.Pointer[lf.Queue[*[]byte]]
	filters []func(
		measurement *data.Measurement,
	) bool
	route  string
	symbol string
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
		filters: filters,
		route:   types.Route(),
		symbol:  types.Focus(),
	}

	tee.ingress.Store(lf.NewQueue[*data.Measurement]())
	tee.egress.Store(lf.NewQueue[*[]byte]())

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

	if measurement == nil {
		return
	}

	for _, filter := range tee.filters {
		if !filter(measurement) {
			return
		}
	}

	inQueue := tee.ingress.Load()
	if inQueue != nil {
		inQueue.Enqueue(measurement)
	}
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

	outQueue := tee.egress.Load()
	if outQueue == nil {
		return nil
	}

	payload, ok := outQueue.Dequeue()

	if !ok || payload == nil || len(*payload) == 0 {
		return nil
	}

	return unsafe.Pointer(payload)
}

/*
IngressLength reports how many measurements are waiting in the ingress queue
to be encoded by the background worker.
*/
func (tee *UITee) IngressLength() int {
	if tee == nil {
		return 0
	}

	inQueue := tee.ingress.Load()
	if inQueue == nil {
		return 0
	}

	return int(inQueue.Length())
}

/*
EgressLength reports how many encoded FlatBuffer frames are queued waiting for
client transmission.
*/
func (tee *UITee) EgressLength() int {
	if tee == nil {
		return 0
	}

	outQueue := tee.egress.Load()
	if outQueue == nil {
		return 0
	}

	return int(outQueue.Length())
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

			inQueue := tee.ingress.Load()
			if inQueue == nil || inQueue.Length() == 0 {
				time.Sleep(10 * time.Millisecond)
				continue
			}

			for inQueue.Length() > 0 {
				measurement, ok := inQueue.Dequeue()

				if !ok {
					break
				}

				dropped := false

				for _, filter := range tee.filters {
					if !filter(measurement) {
						dropped = true
						break
					}
				}

				if dropped {
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

				buf := payload
				outQueue := tee.egress.Load()
				if outQueue != nil {
					outQueue.Enqueue(&buf)
				}
			}
		}
	}
}

func (tee *UITee) switcheroo() {
	currentRoute := types.Route()
	currentFocus := types.Focus()

	if currentRoute != tee.route || currentFocus != tee.symbol {
		errnie.Info("[tee] swapping active queue routing for " + currentRoute + " / " + currentFocus)
		tee.route = currentRoute
		tee.symbol = currentFocus

		tee.ingress.Store(lf.NewQueue[*data.Measurement]())
		tee.egress.Store(lf.NewQueue[*[]byte]())
	}
}
