package view

import (
	"context"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
)

// gaugeInterval rate-limits each source's gauge frame. A gauge shows one number
// per source, so updating it faster than the eye can follow is pure bus and
// websocket load; one update per source per interval is imperceptible.
const gaugeInterval = 200 * time.Millisecond

/*
Gauges turns the measurement stream into the per-source gauge frames the
dashboard reads. Each gauge shows a signal source's strength as its latest SNR,
rate-limited per source. Gauges are not gated by open position — they inform the
entry decision, so they must read before any position exists.
*/
type Gauges struct {
	ctx          context.Context
	cancel       context.CancelFunc
	pool         *qpool.Q
	ui           *qpool.BroadcastGroup
	measurements *qpool.Subscriber
	lastEmit     map[string]time.Time
}

/*
NewGauges builds the gauge feed subscribed to the measurement bus.
*/
func NewGauges(ctx context.Context, pool *qpool.Q) *Gauges {
	ctx, cancel := context.WithCancel(ctx)

	gauges := &Gauges{
		ctx:      ctx,
		cancel:   cancel,
		pool:     pool,
		ui:       pool.CreateBroadcastGroup("ui", 10*time.Millisecond),
		lastEmit: make(map[string]time.Time),
	}

	group := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	gauges.measurements = group.Subscribe("view:gauges", 128)

	return gauges
}

/*
Tick republishes each measurement's source SNR as a gauge frame.
*/
func (gauges *Gauges) Tick() error {
	for {
		select {
		case <-gauges.ctx.Done():
			return gauges.ctx.Err()
		case value, ok := <-gauges.measurements.Incoming:
			if !ok || value == nil || value.Value == nil {
				continue
			}

			measurement, measurementOK := value.Value.(perspectives.Measurement)

			if !measurementOK {
				continue
			}

			gauges.publish(measurement)
		}
	}
}

// publish ships one gauge frame per source per gaugeInterval.
func (gauges *Gauges) publish(measurement perspectives.Measurement) {
	frame, ok := gauges.frame(measurement)

	if !ok {
		return
	}

	source := measurement.Source.String()
	now := time.Now()

	if last, seen := gauges.lastEmit[source]; seen && now.Sub(last) < gaugeInterval {
		return
	}

	gauges.lastEmit[source] = now
	gauges.ui.Send(&qpool.QValue[any]{Value: frame})
}

// frame builds the gauge wire frame for a measurement, reporting ok=false when
// the source has no dashboard name.
func (gauges *Gauges) frame(measurement perspectives.Measurement) (map[string]any, bool) {
	source := measurement.Source.String()

	if source == "" {
		return nil, false
	}

	return map[string]any{
		"source":     source,
		"confidence": perspectives.GaugeValue(measurement),
		"factors":    gaugeFactorsWire(measurement.Factors),
	}, true
}

func gaugeFactorsWire(factors []perspectives.GaugeFactor) []map[string]any {
	if len(factors) == 0 {
		return nil
	}

	wire := make([]map[string]any, len(factors))

	for index, factor := range factors {
		wire[index] = map[string]any{
			"name":  factor.Name,
			"value": factor.Value,
		}
	}

	return wire
}

func (gauges *Gauges) Close() error {
	gauges.cancel()
	return nil
}
