package ui

import (
	"context"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
	"golang.design/x/lockfree/lf"
)

/*
UITee is a concrete off-ramp that accepts *data.Measurement[float64]
and yields encoded FlatBuffer []byte frames for the dashboard websocket,
including ManifoldFrame payloads when a measurement carries a ManifoldState.
It satisfies runtime.Tee[*data.Measurement[float64], []byte].
Encoding is executed in a dedicated background worker to eliminate serialization
jitter from the consumer fast path and immediately release retained publications.
*/
type UITee struct {
	*runtime.System
	ingress             *lf.Queue[data.Publication]
	egress              *lf.Queue[[]byte]
	wake                chan struct{}
	available           chan struct{}
	lastManifoldVersion uint64
	filters             []func(
		measurement *data.Measurement[float64],
	) bool
}

/*
NewUITee creates a new wait-free UITee off-ramp with background FlatBuffer encoding.
*/
func NewUITee(
	ctx context.Context,
	label string,
	filters ...func(
		measurement *data.Measurement[float64],
	) bool,
) *UITee {
	if len(filters) == 0 {
		filters = []func(*data.Measurement[float64]) bool{types.Filters}
	}

	tee := &UITee{
		ingress:   lf.NewQueue[data.Publication](),
		egress:    lf.NewQueue[[]byte](),
		wake:      make(chan struct{}, 1),
		available: make(chan struct{}, 1),
		filters:   filters,
	}

	tee.System = runtime.NewSystem(ctx, label, tee)
	go tee.worker(tee.Context())
	return tee
}

/*
Available returns a receive-only notification channel signaled whenever
a new encoded frame is ready in the egress queue.
*/
func (tee *UITee) Available() <-chan struct{} {
	return tee.available
}

/*
Push receives measurements from the workspace. Raw venue feeds stay off the
dashboard websocket; the page route still selects which analytical sources
are on the wire.
*/
func (tee *UITee) Push(pub data.Publication) {
	if tee.Status() != runtime.READY {
		errnie.Warn("pushing to a non-ready system may have unintended consequences")
		return
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

	select {
	case tee.wake <- struct{}{}:
	default:
	}
}

/*
Next yields the next available pre-encoded FlatBuffer frame ([]byte).
Encoding is owned exclusively by the background worker, so Next is an
instantaneous wait-free dequeue that preserves the worker's encode order.
*/
func (tee *UITee) Next() unsafe.Pointer {
	if tee.Status() != runtime.READY {
		errnie.Warn("[tee] pulling from a non-ready system may have unintended consequences")
		return nil
	}

	payload, ok := tee.egress.Dequeue()

	if !ok || len(payload) == 0 {
		return nil
	}

	return unsafe.Pointer(&payload)
}

func (tee *UITee) worker(ctx context.Context) {
	const batchLimit = 1024
	batch := make([]*data.Measurement[float64], 0, batchLimit)

	for {
		select {
		case <-ctx.Done():
			tee.drainIngress()
			return
		case <-tee.wake:
			for {
				batch = batch[:0]
				var encodedSomething bool
				var dequeuedCount int

				for i := 0; i < batchLimit; i++ {
					pub, ok := tee.ingress.Dequeue()
					if !ok {
						break
					}
					dequeuedCount++

					measurement := pub.Measurement

					if manifoldState, ok := measurement.Result.(*types.ManifoldState); ok {
						if manifoldState != nil && (manifoldState.Version == 0 || manifoldState.Version != tee.lastManifoldVersion) {
							tee.lastManifoldVersion = manifoldState.Version
							payload, err := types.EncodeManifold(manifoldState)
							if err != nil {
								errnie.Error(errnie.Err(
									errnie.UnprocessableContent,
									"[tee] Failed to encode manifold",
									err,
								))
							} else {
								if tee.egress.Length() >= 4096 {
									tee.egress.Dequeue()
								}
								tee.egress.Enqueue(payload)
								encodedSomething = true
							}
						}
						pub.Release()
						continue
					}

					batch = append(batch, measurement)
					pub.Release()
				}

				if len(batch) > 0 {
					payload, err := types.EncodeMeasurements(batch)
					if err != nil {
						errnie.Error(errnie.Err(
							errnie.UnprocessableContent,
							"[tee] Failed to encode measurements",
							err,
						))
					} else {
						if tee.egress.Length() >= 4096 {
							tee.egress.Dequeue()
						}
						tee.egress.Enqueue(payload)
						encodedSomething = true
					}
				}

				if encodedSomething {
					select {
					case tee.available <- struct{}{}:
					default:
					}
				}

				if dequeuedCount == 0 {
					break
				}
			}
		}
	}
}

func (tee *UITee) drainIngress() {
	for {
		pub, ok := tee.ingress.Dequeue()

		if !ok {
			return
		}

		pub.Release()
	}
}
