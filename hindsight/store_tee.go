package hindsight

import (
	"context"
	"math"
	"strconv"
	"unsafe"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
	"golang.design/x/lockfree/lf"
)

const defaultWindowSize = 4096

type symbolState struct {
	cusum         core.Primitive
	excursion     string // "upper", "lower", "chop", "flat"
	startTick     int64
	ignitionTick  int64
	extremumTick  int64
	endTick       int64
	tailTick      int64
	span          int64
	entryPrice    float64
	extremumPrice float64
	exitPrice     float64

	// Noise resistance & stop-loss hunter immunity
	reversalCandTick  int64
	reversalCandPrice float64
	pullbackTicks     int

	// Rolling tape tracking for chop and flat detection
	windowStartTick  int64
	windowStartPrice float64
	windowHigh       float64
	windowLow        float64
	windowTicks      int
	dirFlips         int
	lastDeltaSign    int
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
	price  *broker.Price
	queue  *lf.Queue[*data.Measurement[float64]]
	buffer []*data.Measurement[float64]
	head   int
	tail   int
	count  int
	window int
	states map[string]*symbolState
}

/*
NewStoreTee creates an idle storage off-ramp wired to the broker's Level 3 price and book state.
It flexibly accepts an optional *broker.Price and an optional capacity int.
*/
func NewStoreTee(ctx context.Context, label string, args ...any) *StoreTee {
	var price *broker.Price
	capVal := defaultWindowSize

	for _, arg := range args {
		switch v := arg.(type) {
		case *broker.Price:
			price = v
		case int:
			if v > 0 {
				capVal = v
			}
		}
	}

	tee := &StoreTee{
		price:  price,
		queue:  lf.NewQueue[*data.Measurement[float64]](),
		buffer: make([]*data.Measurement[float64], capVal),
		window: capVal,
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

func (tee *StoreTee) priceAtTick(symbol string, tick int64) float64 {
	for index := 0; index < tee.count; index++ {
		slot := (tee.head + index) % tee.window
		meas := tee.buffer[slot]

		if meas != nil && (meas.Label == symbol || meas.Label == "") && meas.SeqIdx == tick {
			if metric, ok := meas.Metrics["price"]; ok && metric.Raw > 0 {
				return metric.Raw
			}
		}
	}

	if tee.price != nil {
		if mark := tee.price.CurrentMark(symbol); mark != nil && mark.Sign() > 0 {
			return mark.Float64()
		}
	}

	return 0
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
	var spreadVal float64
	var bestAsk float64
	var bestBid float64

	// Authoritative price, book sides, and spread from Level 3 order book via broker.Price
	if tee.price != nil {
		if tee.price.Books != nil {
			tee.price.Books.Book(symbol, func(b *spotbook.Book) {
				if b != nil && b.BestAsk() != nil && b.BestBid() != nil {
					bestAsk = b.BestAsk().Price.Float64()
					bestBid = b.BestBid().Price.Float64()
					s := b.BestAsk().Price.Sub(b.BestBid().Price)
					if s != nil && s.Sign() > 0 {
						spreadVal = s.Float64()
					}
				}
			})
		}

		if mark := tee.price.CurrentMark(symbol); mark != nil && mark.Sign() > 0 {
			priceVal = mark.Float64()
		}
	}

	// Fallback to measurement metrics if price not yet published by Level 3 book
	if priceVal == 0 {
		if metric, ok := measurement.Metrics["price"]; ok && metric.Raw > 0 {
			priceVal = metric.Raw
		}
	}

	if spreadVal == 0 {
		if spreadMetric, hasSpread := measurement.Metrics["spread"]; hasSpread && spreadMetric.Raw > 0 {
			spreadVal = spreadMetric.Raw
		}
	}

	if spreadVal <= 0 && priceVal > 0 {
		spreadVal = priceVal * 0.0002
	}

	if priceVal > 0 {
		// Decorate measurement with the authoritative price
		if _, hasPrice := measurement.Metrics["price"]; !hasPrice {
			m := data.NewMetric[float64]("price", data.Unit("USD"), data.TimescaleInstantaneous, 0, 1)
			m.Raw = priceVal
			measurement.Metrics["price"] = m
		}

		// Exact taker fee from broker.Price (real venue fee rate)
		var feeRate float64
		if tee.price != nil {
			if fee := tee.price.FeeIfAvailable(symbol); fee != nil && fee.Fee != nil {
				feeRate = fee.Fee.Float64() / 100.0 // Fee.Fee is in percent, e.g. 0.26% -> 0.0026
			}
		}

		if feeRate <= 0 {
			feeRate = 0.0026 // Standard 26bp base taker fee default
		}

		hurdle := spreadVal / 2.0
		threshold := spreadVal * 2.0

		if hurdle <= 0 {
			hurdle = priceVal * 0.0001
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

		// Point B Ignition for Upper Excursion (only when idle)
		if state.excursion == "" && reading.Signal == statistic.CUSUMUpper {
			state.excursion = "upper"
			state.startTick = reading.UpperStart
			state.ignitionTick = obs.Sequence

			if bestAsk > 0 {
				state.entryPrice = bestAsk
			} else {
				state.entryPrice = tee.priceAtTick(symbol, state.startTick)
				if state.entryPrice == 0 {
					state.entryPrice = priceVal
				}
			}

			state.extremumPrice = state.entryPrice
			state.extremumTick = obs.Sequence
			state.span = state.ignitionTick - state.startTick

			if state.span < 1 {
				state.span = 1
			}

			leadSpan := state.span * 2
			if leadSpan < 16 {
				leadSpan = 16
			}

			leadTick := state.startTick - leadSpan

			if leadTick < 1 {
				leadTick = 1
			}

			state.endTick = 0
			state.tailTick = 0
			state.reversalCandTick = 0
			state.reversalCandPrice = 0
			state.pullbackTicks = 0
			state.windowTicks = 0 // Reset chop/flat tracking
			tee.tagBuffer(symbol, "upper", state.startTick, leadTick)
		}

		// Point B Ignition for Downward Excursion (only when idle)
		if state.excursion == "" && reading.Signal == statistic.CUSUMLower {
			state.excursion = "lower"
			state.startTick = reading.LowerStart
			state.ignitionTick = obs.Sequence

			if bestBid > 0 {
				state.entryPrice = bestBid
			} else {
				state.entryPrice = tee.priceAtTick(symbol, state.startTick)
				if state.entryPrice == 0 {
					state.entryPrice = priceVal
				}
			}

			state.extremumPrice = state.entryPrice
			state.extremumTick = obs.Sequence
			state.span = state.ignitionTick - state.startTick

			if state.span < 1 {
				state.span = 1
			}

			leadSpan := state.span * 2
			if leadSpan < 16 {
				leadSpan = 16
			}

			leadTick := state.startTick - leadSpan

			if leadTick < 1 {
				leadTick = 1
			}

			state.endTick = 0
			state.tailTick = 0
			state.reversalCandTick = 0
			state.reversalCandPrice = 0
			state.pullbackTicks = 0
			state.windowTicks = 0 // Reset chop/flat tracking
			tee.tagBuffer(symbol, "lower", state.startTick, leadTick)
		}

		// Noise-resistant tracking during active Upper Excursion
		if state.excursion == "upper" {
			if priceVal > state.extremumPrice {
				// New high-water mark reached! Invalidate any pending candidate reversal.
				state.extremumPrice = priceVal
				state.extremumTick = obs.Sequence
				state.reversalCandTick = 0
				state.reversalCandPrice = 0
				state.pullbackTicks = 0
			} else if state.ignitionTick > 0 && obs.Sequence > state.ignitionTick && state.endTick == 0 {
				drop := state.extremumPrice - priceVal
				gain := state.extremumPrice - state.entryPrice

				isCandidate := false
				if gain > 0 {
					// 38.2% Fibonacci pull-back or drop exceeding 2*hurdle or opposite CUSUM
					if drop >= (0.382 * gain) || drop >= (2.0 * hurdle) || reading.Signal == statistic.CUSUMLower || priceVal <= state.entryPrice-hurdle {
						isCandidate = true
					}
				} else if priceVal <= state.entryPrice-hurdle {
					isCandidate = true
				}

				if isCandidate {
					if state.reversalCandTick == 0 {
						// First tick of pullback: mark candidate tick and price
						state.reversalCandTick = obs.Sequence
						state.reversalCandPrice = priceVal
						state.pullbackTicks = 1
					} else {
						// Next tick: is it continuing or staying down?
						if priceVal <= state.reversalCandPrice+(hurdle*0.5) || reading.Signal == statistic.CUSUMLower {
							state.pullbackTicks++
						} else {
							// Price rebounded back up: it was just a 1-tick stop-loss hunter / wick!
							state.reversalCandTick = 0
							state.reversalCandPrice = 0
							state.pullbackTicks = 0
						}
					}

					// Confirm Point C only when sustained (>=2 ticks)
					if state.pullbackTicks >= 2 {
						state.endTick = state.reversalCandTick
						if bestBid > 0 {
							state.exitPrice = bestBid
						} else {
							state.exitPrice = priceVal
						}
						if state.exitPrice <= 0 {
							state.exitPrice = priceVal
						}
						runSpan := state.extremumTick - state.ignitionTick
						tailMargin := state.span
						if runSpan/2 > tailMargin {
							tailMargin = runSpan / 2
						}
						state.tailTick = obs.Sequence + tailMargin
					}
				} else {
					if state.reversalCandTick > 0 {
						// Recovered: reset candidate
						state.reversalCandTick = 0
						state.reversalCandPrice = 0
						state.pullbackTicks = 0
					}
				}
			}
		}

		// Noise-resistant tracking during active Downward Excursion
		if state.excursion == "lower" {
			if priceVal < state.extremumPrice || state.extremumPrice == 0 {
				// New low-water mark reached!
				state.extremumPrice = priceVal
				state.extremumTick = obs.Sequence
				state.reversalCandTick = 0
				state.reversalCandPrice = 0
				state.pullbackTicks = 0
			} else if state.ignitionTick > 0 && obs.Sequence > state.ignitionTick && state.endTick == 0 {
				bounce := priceVal - state.extremumPrice
				drop := state.entryPrice - state.extremumPrice

				isCandidate := false
				if drop > 0 {
					if bounce >= (0.382 * drop) || bounce >= (2.0 * hurdle) || reading.Signal == statistic.CUSUMUpper || priceVal >= state.entryPrice+hurdle {
						isCandidate = true
					}
				} else if priceVal >= state.entryPrice+hurdle {
					isCandidate = true
				}

				if isCandidate {
					if state.reversalCandTick == 0 {
						state.reversalCandTick = obs.Sequence
						state.reversalCandPrice = priceVal
						state.pullbackTicks = 1
					} else {
						if priceVal >= state.reversalCandPrice-(hurdle*0.5) || reading.Signal == statistic.CUSUMUpper {
							state.pullbackTicks++
						} else {
							state.reversalCandTick = 0
							state.reversalCandPrice = 0
							state.pullbackTicks = 0
						}
					}

					// Confirm Point C only when sustained (>=2 ticks)
					if state.pullbackTicks >= 2 {
						state.endTick = state.reversalCandTick
						if bestAsk > 0 {
							state.exitPrice = bestAsk
						} else {
							state.exitPrice = priceVal
						}
						if state.exitPrice <= 0 {
							state.exitPrice = priceVal
						}
						runSpan := state.extremumTick - state.ignitionTick
						tailMargin := state.span
						if runSpan/2 > tailMargin {
							tailMargin = runSpan / 2
						}
						state.tailTick = obs.Sequence + tailMargin
					}
				} else {
					if state.reversalCandTick > 0 {
						state.reversalCandTick = 0
						state.reversalCandPrice = 0
						state.pullbackTicks = 0
					}
				}
			}
		}

		// Rolling evaluation for Chop and Flat lines when no directional excursion is active
		if state.excursion == "" && priceVal > 0 {
			if state.windowTicks == 0 {
				state.windowStartTick = obs.Sequence
				state.windowStartPrice = priceVal
				state.windowHigh = priceVal
				state.windowLow = priceVal
				state.windowTicks = 1
				state.dirFlips = 0
				state.lastDeltaSign = 0
			} else {
				state.windowTicks++
				if priceVal > state.windowHigh {
					state.windowHigh = priceVal
				}
				if priceVal < state.windowLow {
					state.windowLow = priceVal
				}

				delta := priceVal - state.windowStartPrice
				deltaSign := 0
				if delta > 0 {
					deltaSign = 1
				}
				if delta < 0 {
					deltaSign = -1
				}
				if deltaSign != 0 && state.lastDeltaSign != 0 && deltaSign != state.lastDeltaSign {
					state.dirFlips++
				}
				if deltaSign != 0 {
					state.lastDeltaSign = deltaSign
				}

				if state.windowTicks >= 32 {
					rangeVal := state.windowHigh - state.windowLow

					// Scenario 5: Flat line (near-zero range over 32 ticks)
					if rangeVal <= 1.0*hurdle || (priceVal > 0 && rangeVal/priceVal < 0.0003) {
						state.excursion = "flat"
						state.startTick = state.windowStartTick
						state.ignitionTick = state.windowStartTick + int64(state.windowTicks/2)
						state.extremumTick = state.ignitionTick
						state.endTick = obs.Sequence
						state.tailTick = obs.Sequence
						state.entryPrice = state.windowStartPrice
						state.extremumPrice = (state.windowHigh + state.windowLow) / 2.0
						state.exitPrice = priceVal
						state.span = int64(state.windowTicks / 2)
						tee.tagBuffer(symbol, "flat", state.startTick, state.startTick)
						tee.completeExcursion(measurement, state, feeRate)
						state.windowTicks = 0
					} else if state.dirFlips >= 6 && rangeVal <= threshold {
						// Scenario 4: Chop (oscillating noise trapped inside friction band)
						state.excursion = "chop"
						state.startTick = state.windowStartTick
						state.ignitionTick = state.windowStartTick + int64(state.windowTicks/2)
						state.extremumTick = state.ignitionTick
						state.endTick = obs.Sequence
						state.tailTick = obs.Sequence
						state.entryPrice = state.windowStartPrice
						state.extremumPrice = state.windowHigh
						state.exitPrice = priceVal
						state.span = int64(state.windowTicks / 2)
						tee.tagBuffer(symbol, "chop", state.startTick, state.startTick)
						tee.completeExcursion(measurement, state, feeRate)
						state.windowTicks = 0
					} else {
						// Rolling slide
						state.windowTicks = 16
						state.windowStartTick = obs.Sequence - 16
						state.windowStartPrice = priceVal
						state.windowHigh = priceVal
						state.windowLow = priceVal
						state.dirFlips = 0
					}
				}
			}
		}

		// Check if tail margin has completed for active excursion
		if state.tailTick > 0 && obs.Sequence >= state.tailTick {
			tee.completeExcursion(measurement, state, feeRate)
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

func (tee *StoreTee) completeExcursion(
	measurement *data.Measurement[float64],
	state *symbolState,
	feeRate float64,
) {
	const positionSize = 40.0 // Realistic $40 position
	var grossExcursion, profit, profitFraction, totalFee float64
	clearsFriction := false
	category := ""

	if state.entryPrice > 0 {
		entryCash := positionSize * (1.0 + feeRate)
		var exitCash float64

		if state.excursion == "upper" {
			grossExcursion = (state.extremumPrice - state.entryPrice) / state.entryPrice
			exitCash = positionSize * (state.exitPrice / state.entryPrice) * (1.0 - feeRate)
			totalFee = (positionSize * feeRate) + (positionSize * (state.exitPrice / state.entryPrice) * feeRate)
			profit = exitCash - entryCash
			profitFraction = profit / entryCash

			// Must be strictly profitable on a $40 position after taker fees and spread
			if profit > 0 {
				clearsFriction = true
				category = "upper_profitable"
			} else {
				clearsFriction = false
				category = "upper_unprofitable"
			}
		} else if state.excursion == "lower" {
			grossExcursion = (state.entryPrice - state.extremumPrice) / state.entryPrice
			exitCash = positionSize * (state.exitPrice / state.entryPrice) * (1.0 - feeRate)
			totalFee = (positionSize * feeRate) + (positionSize * (state.exitPrice / state.entryPrice) * feeRate)
			profit = exitCash - entryCash
			profitFraction = profit / entryCash
			clearsFriction = false
			category = "downward"
		} else if state.excursion == "chop" {
			grossExcursion = math.Abs(state.extremumPrice-state.entryPrice) / state.entryPrice
			exitCash = positionSize * (state.exitPrice / state.entryPrice) * (1.0 - feeRate)
			totalFee = (positionSize * feeRate) + (positionSize * (state.exitPrice / state.entryPrice) * feeRate)
			profit = exitCash - entryCash
			profitFraction = profit / entryCash
			clearsFriction = false
			category = "chop"
		} else if state.excursion == "flat" {
			grossExcursion = math.Abs(state.extremumPrice-state.entryPrice) / state.entryPrice
			exitCash = positionSize * (state.exitPrice / state.entryPrice) * (1.0 - feeRate)
			totalFee = (positionSize * feeRate) + (positionSize * (state.exitPrice / state.entryPrice) * feeRate)
			profit = exitCash - entryCash
			profitFraction = profit / entryCash
			clearsFriction = false
			category = "flat"
		}
	}

	if measurement.Metadata == nil {
		measurement.Metadata = make(map[string]string)
	}

	measurement.Metadata["excursion_event"] = "completed"
	measurement.Metadata["excursion"] = state.excursion
	measurement.Metadata["excursion_category"] = category
	measurement.Metadata["excursion_start"] = strconv.FormatInt(state.startTick, 10)
	measurement.Metadata["excursion_ignition"] = strconv.FormatInt(state.ignitionTick, 10)
	measurement.Metadata["excursion_extremum_tick"] = strconv.FormatInt(state.extremumTick, 10)
	measurement.Metadata["excursion_end"] = strconv.FormatInt(state.endTick, 10)
	measurement.Metadata["excursion_entry_price"] = strconv.FormatFloat(state.entryPrice, 'g', -1, 64)
	measurement.Metadata["excursion_extremum_price"] = strconv.FormatFloat(state.extremumPrice, 'g', -1, 64)
	measurement.Metadata["excursion_exit_price"] = strconv.FormatFloat(state.exitPrice, 'g', -1, 64)
	measurement.Metadata["excursion_position_size"] = strconv.FormatFloat(positionSize, 'g', -1, 64)
	measurement.Metadata["excursion_fee"] = strconv.FormatFloat(totalFee, 'g', -1, 64)
	measurement.Metadata["excursion_profit"] = strconv.FormatFloat(profit, 'g', -1, 64)
	measurement.Metadata["excursion_profit_fraction"] = strconv.FormatFloat(profitFraction, 'g', -1, 64)
	measurement.Metadata["excursion_gross"] = strconv.FormatFloat(grossExcursion, 'g', -1, 64)
	measurement.Metadata["excursion_clears_friction"] = strconv.FormatBool(clearsFriction)

	// Reset state for next excursion
	state.cusum = statistic.NewCUSUM()
	state.excursion = ""
	state.startTick = 0
	state.ignitionTick = 0
	state.extremumTick = 0
	state.endTick = 0
	state.tailTick = 0
	state.entryPrice = 0
	state.extremumPrice = 0
	state.exitPrice = 0
	state.reversalCandTick = 0
	state.reversalCandPrice = 0
	state.pullbackTicks = 0
	state.windowTicks = 0
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
