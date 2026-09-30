package hindsight

import (
	"context"
	"strconv"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
	"golang.design/x/lockfree/lf"
)

const defaultWindowSize = 4096

type symbolState struct {
	cusum        core.Primitive
	excursion    string
	startTick    int64
	ignitionTick int64
	endTick      int64
	tailTick     int64
	span         int64
}

/*
StoreTee queues measurements for the catalog drain. It implements runtime.Tee
and remains idle until startup explicitly transitions it to READY. Its lock-free
queue accepts concurrent workspace consumers without waiting for the catalog drain.
Observations are buffered in Next to allow complete precursor and tail margin
tagging across all signal and logic stages before emitting to storage.
*/
type StoreTee struct {
	*runtime.System
	queue  *lf.Queue[*data.Measurement[float64]]
	buffer []*data.Measurement[float64]
	head   int
	tail   int
	count  int
	window int
	states map[string]*symbolState
}

/*
NewStoreTee creates an idle storage off-ramp.
*/
func NewStoreTee(ctx context.Context, label string, _ ...int) *StoreTee {
	tee := &StoreTee{
		queue:  lf.NewQueue[*data.Measurement[float64]](),
		buffer: make([]*data.Measurement[float64], defaultWindowSize),
		window: defaultWindowSize,
		states: make(map[string]*symbolState),
	}

	tee.System = runtime.NewSystem(ctx, label, tee)
	return tee
}

/*
Push receives measurements from the workspace after startup opens the tee.
*/
func (tee *StoreTee) Push(measurement *data.Measurement[float64]) {
	if tee.Status() != runtime.READY {
		errnie.Warn("pushing to a non-ready system may have unintended consequences")
		return
	}

	if measurement == nil {
		return
	}

	clone := measurement.Clone()
	clone.Peers = nil
	tee.queue.Enqueue(clone)
}

func (tee *StoreTee) tag(measurement *data.Measurement[float64]) {
	symbol := measurement.Label

	if symbol == "" {
		symbol = "default"
	}

	state := tee.states[symbol]

	if state == nil {
		state = &symbolState{cusum: statistic.NewCUSUM()}
		tee.states[symbol] = state
	}

	var priceVal float64
	for _, key := range []string{"price", "last_price", "last", "spot_price", "reference_price", "midpoint"} {
		if metric, ok := measurement.Metrics[key]; ok && metric.Raw > 0 {
			priceVal = metric.Raw
			break
		}
	}

	if priceVal > 0 {
		var hurdle float64
		var threshold float64

		if spreadMetric, hasSpread := measurement.Metrics["spread"]; hasSpread && spreadMetric.Raw > 0 {
			hurdle = spreadMetric.Raw / 2.0
			threshold = spreadMetric.Raw * 2.0
		}

		if hurdle == 0 {
			if relSpread, has := measurement.Metrics["relative_spread"]; has && relSpread.Raw > 0 {
				spreadVal := relSpread.Raw * priceVal
				hurdle = spreadVal / 2.0
				threshold = spreadVal * 2.0
			}
		}

		if hurdle == 0 && measurement.SNRDefined && measurement.SNR > 0 {
			hurdle = priceVal / measurement.SNR
			threshold = hurdle * 4.0
		}

		obs := statistic.CUSUMObservation{
			Sequence:  measurement.SeqIdx,
			Value:     priceVal,
			Hurdle:    hurdle,
			Threshold: threshold,
		}

		var reading statistic.CUSUMReading

		for out := range state.cusum.Next(transport.NewOne(unsafe.Pointer(&obs)).Next(nil)) {
			reading = *(*statistic.CUSUMReading)(out)
		}

		if measurement.Metadata == nil {
			measurement.Metadata = make(map[string]string)
		}

		measurement.Metadata["cusum_upper"] = strconv.FormatFloat(reading.UpperSum, 'g', -1, 64)
		measurement.Metadata["cusum_lower"] = strconv.FormatFloat(reading.LowerSum, 'g', -1, 64)

		if reading.Signal == statistic.CUSUMUpper {
			state.excursion = "upper"
			state.startTick = reading.UpperStart
			state.ignitionTick = obs.Sequence
			state.span = state.ignitionTick - state.startTick

			if state.span < 1 {
				state.span = 1
			}

			leadTick := state.startTick - state.span

			if leadTick < 1 {
				leadTick = 1
			}

			state.endTick = 0
			state.tailTick = 0
			tee.tagBuffer(symbol, "upper", state.startTick, leadTick)
		}

		if reading.Signal == statistic.CUSUMLower {
			state.excursion = "lower"
			state.startTick = reading.LowerStart
			state.ignitionTick = obs.Sequence
			state.span = state.ignitionTick - state.startTick

			if state.span < 1 {
				state.span = 1
			}

			leadTick := state.startTick - state.span

			if leadTick < 1 {
				leadTick = 1
			}

			state.endTick = 0
			state.tailTick = 0
			tee.tagBuffer(symbol, "lower", state.startTick, leadTick)
		}

		// Detect exhaustion / reversal (Point C)
		if state.excursion == "upper" && state.ignitionTick > 0 && obs.Sequence > state.ignitionTick {
			if reading.LowerSum < 0 || reading.UpperSum == 0 {
				state.endTick = obs.Sequence
				state.tailTick = state.endTick + state.span
			}
		}

		if state.excursion == "lower" && state.ignitionTick > 0 && obs.Sequence > state.ignitionTick {
			if reading.UpperSum > 0 || reading.LowerSum == 0 {
				state.endTick = obs.Sequence
				state.tailTick = state.endTick + state.span
			}
		}

		// Check if tail margin has completed
		if state.tailTick > 0 && obs.Sequence >= state.tailTick {
			measurement.Metadata["excursion_event"] = "completed"
			measurement.Metadata["excursion"] = state.excursion
			measurement.Metadata["excursion_start"] = strconv.FormatInt(state.startTick, 10)
			measurement.Metadata["excursion_ignition"] = strconv.FormatInt(state.ignitionTick, 10)
			measurement.Metadata["excursion_end"] = strconv.FormatInt(state.endTick, 10)

			state.excursion = ""
			state.startTick = 0
			state.ignitionTick = 0
			state.endTick = 0
			state.tailTick = 0
		}
	}

	if state.excursion != "" {
		if measurement.Metadata == nil {
			measurement.Metadata = make(map[string]string)
		}

		measurement.Metadata["excursion"] = state.excursion
		measurement.Metadata["excursion_start"] = strconv.FormatInt(state.startTick, 10)
		measurement.Metadata["excursion_ignition"] = strconv.FormatInt(state.ignitionTick, 10)
		if state.endTick > 0 {
			measurement.Metadata["excursion_end"] = strconv.FormatInt(state.endTick, 10)
		}
	}
}

func (tee *StoreTee) tagBuffer(symbol, excursion string, startTick, leadTick int64) {
	for index := 0; index < tee.count; index++ {
		slot := (tee.head + index) % tee.window
		meas := tee.buffer[slot]

		if meas == nil {
			continue
		}

		if (meas.Label == symbol || meas.Label == "") && meas.SeqIdx >= leadTick {
			if meas.Metadata == nil {
				meas.Metadata = make(map[string]string)
			}

			meas.Metadata["excursion"] = excursion
			meas.Metadata["excursion_start"] = strconv.FormatInt(startTick, 10)
		}
	}
}

func (tee *StoreTee) pushBuffer(measurement *data.Measurement[float64]) {
	if tee.count == tee.window {
		tee.buffer[tee.head] = measurement
		tee.head = (tee.head + 1) % tee.window
		tee.tail = (tee.tail + 1) % tee.window
		return
	}

	tee.buffer[tee.tail] = measurement
	tee.tail = (tee.tail + 1) % tee.window
	tee.count++
}

func (tee *StoreTee) popBuffer() *data.Measurement[float64] {
	if tee.count == 0 {
		return nil
	}

	measurement := tee.buffer[tee.head]
	tee.buffer[tee.head] = nil
	tee.head = (tee.head + 1) % tee.window
	tee.count--
	return measurement
}

/*
Next returns a measurement pointer, or nil while idle or when the queue is empty.
*/
func (tee *StoreTee) Next() unsafe.Pointer {
	if tee.Status() != runtime.READY {
		errnie.Warn(tee.Name() + ": Next called before READY; dropping event")
		return nil
	}

	for tee.count < tee.window {
		measurement, ok := tee.queue.Dequeue()

		if !ok {
			break
		}

		tee.tag(measurement)
		tee.pushBuffer(measurement)
	}

	if tee.count == 0 {
		return nil
	}

	measurement := tee.popBuffer()
	return unsafe.Pointer(measurement)
}

// Pending reports accepted observations waiting for the catalog drain.
func (tee *StoreTee) Pending() int {
	return int(tee.queue.Length()) + tee.count
}
