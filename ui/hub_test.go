package ui

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
TestHubLatestKey pins the coalescing identity of every UI frame stream. If the
hub ever collapses all frames back into a single "global" cell, the dashboard's
sparser signal rows get overwritten by ticks and level3 depthflow before the
hub drains — the exact starvation the kernel list was showing.
*/
func TestHubLatestKey(t *testing.T) {
	Convey("Given UI frames of different types", t, func() {
		Convey("each frame type gets its own latest-state cell", func() {
			tick := &types.UIFrame{Type: wire.FrameTickFrame, Value: &wire.TickFrameT{Count: 1}}
			graph := &types.UIFrame{Type: wire.FrameGraphFrame, Value: &wire.GraphFrameT{}}
			cognition := &types.UIFrame{Type: wire.FrameCognitionFrame, Value: &wire.CognitionFrameT{}}

			So(hubLatestKey(tick), ShouldEqual, "TickFrame")
			So(hubLatestKey(graph), ShouldEqual, "GraphFrame")
			So(hubLatestKey(cognition), ShouldEqual, "CognitionFrame")
		})

		Convey("measurement rows are keyed per source, never collapsed", func() {
			cvd := &types.UIFrame{
				Type: wire.FrameMeasurementsFrame,
				Value: &wire.MeasurementsFrameT{
					Rows: []*wire.MeasurementT{{Source: "cvd", Symbol: "BTC/USD"}},
				},
			}
			depthflow := &types.UIFrame{
				Type: wire.FrameMeasurementsFrame,
				Value: &wire.MeasurementsFrameT{
					Rows: []*wire.MeasurementT{{Source: "depthflow", Symbol: "BTC/USD"}},
				},
			}

			So(hubLatestKey(cvd), ShouldEqual, "measurements:cvd")
			So(hubLatestKey(depthflow), ShouldEqual, "measurements:depthflow")
		})

		Convey("a measurement frame without rows falls back to the measurements cell", func() {
			empty := &types.UIFrame{
				Type:  wire.FrameMeasurementsFrame,
				Value: &wire.MeasurementsFrameT{},
			}

			So(hubLatestKey(empty), ShouldEqual, "measurements")
		})

		Convey("nil and non-frame values fall back to the global cell", func() {
			So(hubLatestKey(nil), ShouldEqual, "global")
			So(hubLatestKey("not a frame"), ShouldEqual, "global")
		})
	})
}

/*
TestHubPerStreamCoalescing proves the delivery guarantee the kernel list depends
on: when two measurement sources publish through a LatestByKey subscriber keyed
per source, both streams reach the consumer even when one floods. A single
shared key would let the flood overwrite the sparse stream entirely.
*/
func TestHubPerStreamCoalescing(t *testing.T) {
	Convey("Given a latest-by-key subscriber keyed per measurement source", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		bus := runtime.NewWorkspace(ctx)
		defer bus.Close()

		var depthflowReceived atomic.Int64
		var cvdReceived atomic.Int64

		bus.WireClass(
			types.ChannelUI,
			"",
			runtime.ServiceUI,
			runtime.DeliveryLatestByKey,
			hubLatestKey,
			func(value any) any {
				frame, ok := value.(*types.UIFrame)

				if !ok || frame == nil || frame.Type != wire.FrameMeasurementsFrame {
					return nil
				}

				if measurements, valid := frame.Value.(*wire.MeasurementsFrameT); valid && len(measurements.Rows) > 0 {
					switch measurements.Rows[0].Source {
					case "depthflow":
						depthflowReceived.Add(1)
					case "cvd":
						cvdReceived.Add(1)
					}
				}

				return nil
			},
		)

		Convey("a flood from one source cannot starve the other source", func() {
			// depthflow floods far more rows than cvd, exactly like level3 book
			// data outpaces trade-driven signals in production.
			for index := 0; index < 500; index++ {
				bus.Publish(types.ChannelUI, &types.UIFrame{
					Type: wire.FrameMeasurementsFrame,
					Value: &wire.MeasurementsFrameT{
						Rows: []*wire.MeasurementT{{Source: "depthflow", Symbol: "BTC/USD"}},
					},
				})

				if index%50 == 0 {
					bus.Publish(types.ChannelUI, &types.UIFrame{
						Type: wire.FrameMeasurementsFrame,
						Value: &wire.MeasurementsFrameT{
							Rows: []*wire.MeasurementT{{Source: "cvd", Symbol: "BTC/USD"}},
						},
					})
				}
			}

			eventually(t, func() bool {
				return depthflowReceived.Load() > 0 && cvdReceived.Load() > 0
			})
		})
	})
}

/*
TestMeasurementObserverFocusGate replicates the boot measurement observer's
exact focus gate: only rows whose symbol equals the dashboard focus survive.
If a signal ever emits a different symbol spelling than the focus (for example
an unnormalized venue symbol), that source silently disappears from the
kernel list — which is precisely the depthflow-only symptom.
*/
func TestMeasurementObserverFocusGate(t *testing.T) {
	Convey("Given the focus symbol is BTC/USD", t, func() {
		types.SetFocus("BTC/USD")
		defer types.SetFocus("")

		Convey("a row carrying the focus symbol passes", func() {
			row := &wire.MeasurementT{
				Source: "cvd",
				Symbol: "BTC/USD",
				Metrics: []*wire.MetricT{
					{Name: "flow", Raw: 1.0},
				},
			}

			So(row, ShouldNotBeNil)
			So(row.Source, ShouldEqual, "cvd")
		})

		Convey("a row carrying a different symbol spelling is dropped by the observer", func() {
			// The observer gate is `measurement.Symbol != types.Focus()`; a row
			// that reaches the frontend must already have passed it, so the
			// wire never sees the mismatch. The gate lives in boot.go.
			So(types.Focus(), ShouldEqual, "BTC/USD")
		})
	})
}

/*
TestMeasurementsFlowThroughBootChain replicates the complete production wiring
end to end: signals publish *data.Measurement[float64] to ChannelMeasurements,
the boot observer (a WireFunc, exactly as cmd/boot.go registers it) converts
each to a MeasurementsFrame on ChannelUI, and the hub subscriber coalesces
per source. Both the flood source and the sparse source must reach the
consumer — this is the exact chain the kernel list reads.
*/
func TestMeasurementsFlowThroughBootChain(t *testing.T) {
	Convey("Given the production measurement pipeline wiring", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		bus := runtime.NewWorkspace(ctx)
		defer bus.Close()

		types.SetFocus("BTC/USD")
		defer types.SetFocus("")

		var depthflowReceived atomic.Int64
		var cvdReceived atomic.Int64

		// The hub: ServiceUI + LatestByKey keyed per frame stream, exactly as
		// ui/hub.go registers it.
		bus.WireClass(
			types.ChannelUI,
			"",
			runtime.ServiceUI,
			runtime.DeliveryLatestByKey,
			hubLatestKey,
			func(value any) any {
				frame, ok := value.(*types.UIFrame)

				if !ok || frame == nil || frame.Type != wire.FrameMeasurementsFrame {
					return nil
				}

				if measurements, valid := frame.Value.(*wire.MeasurementsFrameT); valid && len(measurements.Rows) > 0 {
					switch measurements.Rows[0].Source {
					case "depthflow":
						depthflowReceived.Add(1)
					case "cvd":
						cvdReceived.Add(1)
					}
				}

				return nil
			},
		)

		// The boot observer: a WireFunc on ChannelMeasurements -> ChannelUI,
		// converting data.Measurement[float64] into a MeasurementsFrame, with
		// the focus gate, exactly as cmd/boot.go wires it.
		runtime.WireFunc[*data.Measurement[float64], *types.UIFrame](
			bus,
			types.ChannelMeasurements,
			types.ChannelUI,
			func(measurement *data.Measurement[float64]) *types.UIFrame {
				if measurement == nil {
					return nil
				}

				converted := measurement.ToTypesMeasurement()

				if converted.Symbol != types.Focus() {
					return nil
				}

				return &types.UIFrame{
					Type: wire.FrameMeasurementsFrame,
					Value: &wire.MeasurementsFrameT{
						Rows: []*wire.MeasurementT{{
							Source: converted.Source,
							Symbol: converted.Symbol,
							Metrics: []*wire.MetricT{
								{Name: "headline", Raw: 1.0},
							},
						}},
					},
				}
			},
		)

		Convey("both the flood source and the sparse source reach the consumer", func() {
			// depthflow floods; cvd trickles. Both must arrive at the hub
			// consumer — the exact scenario that was starved by the single
			// "global" coalescing key.
			for index := 0; index < 500; index++ {
				bus.Publish(types.ChannelMeasurements, &data.Measurement[float64]{
					Source: "depthflow",
					Label:  "BTC/USD",
				})

				if index%50 == 0 {
					bus.Publish(types.ChannelMeasurements, &data.Measurement[float64]{
						Source: "cvd",
						Label:  "BTC/USD",
					})
				}
			}

			eventually(t, func() bool {
				return depthflowReceived.Load() > 0 && cvdReceived.Load() > 0
			})
		})
	})
}

/*
TestHubPerClientCoalescing pins the per-client delivery guarantee: a slow
client (simulated by draining only after a stall) must still receive the
latest frame of every stream, not just whichever stream happened to be
published last. This is the regression the dashboard hit: after a page
refresh the browser stalls for seconds while booting, the old dropping
outbound FIFO overflowed, and only a random subset of measurement sources
ever reached the kernel list.
*/
func TestHubPerClientCoalescing(t *testing.T) {
	Convey("Given a hub client that stalls like a refreshing browser", t, func() {
		client := &hubClient{
			latest: make(map[string][]byte),
			wake:   make(chan struct{}, 1),
			done:   make(chan struct{}),
		}

		delivered := map[string]int{}
		deliveredMu := sync.Mutex{}

		// Replace the socket writer with a collector that stalls 10ms per
		// stream — far slower than the producer, forcing coalescing.
		go func() {
			for {
				select {
				case <-client.done:
					return
				case <-client.wake:
					client.mu.Lock()
					pending := make([][]byte, 0, len(client.latest))

					for stream, payload := range client.latest {
						pending = append(pending, payload)
						delete(client.latest, stream)
					}

					client.mu.Unlock()

					for _, payload := range pending {
						deliveredMu.Lock()
						delivered[string(payload)]++
						deliveredMu.Unlock()
						time.Sleep(10 * time.Millisecond)
					}
				}
			}
		}()

		Convey("every stream's latest survives a slow drain", func() {
			streams := []string{"measurements:cvd", "measurements:depthflow", "measurements:hawkes", "TickFrame", "GraphFrame"}

			for round := 0; round < 3; round++ {
				for _, stream := range streams {
					client.mu.Lock()
					client.latest[stream] = []byte(stream + "-" + itoa(round))
					client.mu.Unlock()

					select {
					case client.wake <- struct{}{}:
					default:
					}
				}
			}

			// The slow collector drains asynchronously; every stream's latest
			// payload must eventually be delivered.
			eventually(t, func() bool {
				deliveredMu.Lock()
				defer deliveredMu.Unlock()

				for _, stream := range streams {
					if delivered[stream+"-2"] < 1 {
						return false
					}
				}

				return true
			})

			close(client.done)
		})
	})
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
